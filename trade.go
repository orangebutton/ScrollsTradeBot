package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

type Price struct{ Buy, Sell int }

var SGPrices = make(map[Card]Price)

var Gold int

var TradeRoom Channel

type TradeStatus struct {
	Partner Player
	Updated bool
	Their   struct {
		Value    int
		Cards    map[Card]int
		Gold     int
		Accepted bool
	}
	My struct {
		Value    int
		Cards    map[Card]int
		Gold     int
		Accepted bool
	}
}

func GoldForTrade() int {
	return Gold / Conf.GoldDivisor
}

func LoadPrices() {
	resp, err := http.Get("http://a.scrollsguide.com/prices")
	deny(err)
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	deny(err)

	type JSON struct {
		Msg  string
		Data []struct {
			Id       CardId
			Buy      int
			Sell     int
			LastSeen int
		}
		ApiVersion int
	}

	var v JSON
	err = json.Unmarshal(body, &v)
	deny(err)

	for id, name := range CardTypes {
		p := Price{Buy: MinimumValue(name), Sell: MaximumValue(name)}
		for _, data := range v.Data {
			if data.Id == id {
				if data.Buy > p.Buy {
					p.Buy = data.Buy
				}
				if data.Sell < p.Sell {
					p.Sell = data.Sell
				}
			}
		}
		if p.Sell < p.Buy {
			t := (p.Sell + p.Buy) / 2
			p.Sell, p.Buy = t, t
		}
		SGPrices[name] = p
	}
}

func MinimumValue(card Card) int {
	switch CardRarities[card] {
	case 0:
		return 25
	case 1:
		return 50
	case 2:
		return 100
	}
	return -1
}

func MaximumValue(card Card) int {
	switch CardRarities[card] {
	case 0:
		return 150
	case 1:
		return 600
	case 2:
		return 1200
	}
	return -1
}

func (s *State) DeterminePrice(card Card, num int, buy bool) int {
	if Conf.UseScrollsGuidePrice {
		if buy {
			return SGPrices[card].Buy * num
		} else {
			return SGPrices[card].Sell * num
		}
	} else {
		// return clockworkPricing(card, num, buy)
		return autobotsPricing(card, num, buy)
	}
}

func pricingBasedOnInventory(card Card, num int, buy bool) int {
	price := 0
	stocked := Stocks[Bot][card]

	value := func(card Card, stocked int) float64 {
		basePrice := float64(MaximumValue(card))
		return basePrice * (1.0 - 1./Conf.MaxNumToBuy*float64(stocked))
	}

	goldFactor := math.Min(float64(GoldForTrade()), Conf.GoldThreshold)/(Conf.GoldThreshold*2) + 0.5
	for i := 0; i < num; i++ {
		if buy {
			price += int(math.Max(float64(MinimumValue(card)), value(card, stocked)*goldFactor))
			stocked++
		} else {
			price += int(math.Max(float64(MinimumValue(card)), value(card, stocked)*1.00))
			stocked--
		}
	}
	return price
}

func autobotsPricing(card Card, num int, buy bool) int {
	price := 0
	if buy {
		price = num * MinimumValue(card)
	} else {
		price = pricingBasedOnInventory(card, num, buy)
	}
	return price
}

func (s *State) ParseTradeResponse(v MTradeResponse) {
	if v.Status == "DECLINE" {
		s.chTradeResponse <- false
	} else {
		s.chTradeResponse <- true
	}
}

func (s *State) ParseTradeView(v MTradeView) {
	my := v.From
	their := v.To
	tradePartner := v.To.Profile.Name
	if their.Profile.Id == PlayerIds[Bot] {
		my, their = their, my
		tradePartner = v.From.Profile.Name
	}

	convertAndCount := func(cardIds []CardUid, player Player) map[Card]int {
		count := make(map[Card]int)
		for _, id := range cardIds {
			for _, card := range Libraries[player].Cards {
				if card.Id == id {
					cardName := CardTypes[card.TypeId]
					count[cardName] = count[cardName] + 1
					break
				}
			}
		}
		return count
	}

	ts := TradeStatus{}
	ts.Updated = v.Modified
	ts.Partner = tradePartner
	ts.Their.Accepted = their.Accepted
	ts.Their.Cards = convertAndCount(their.CardIds, tradePartner)
	ts.Their.Gold = their.Gold
	ts.My.Accepted = my.Accepted
	ts.My.Cards = convertAndCount(my.CardIds, Bot)
	ts.My.Gold = my.Gold

	s.chTradeStatus <- ts
}

func (s *State) InitiateTrade(player Player, timeout time.Duration) chan TradeStatus {
	s.SendRequest(Request{"msg": "TradeInvite", "profile": PlayerIds[player]})
	accepted := false
	TradeRoom = ""

	cancel := time.After(timeout)
	l := s.Listen()
	defer s.Shut(l)

	for {
		if TradeRoom != "" && accepted {
			break
		}

		select {
		case ok := <-s.chTradeResponse:
			if !ok { // they rejected the trade invite
				log.Printf("REJECT")
				return nil
			} else {
				accepted = true
			}
		case m := <-l: // find out what room we're trading in
			if m.From == "Scrolls" && strings.HasPrefix(string(m.Channel), "trade-") && strings.HasPrefix(m.Text, "You have joined") {
				log.Printf("ACCEPT")
				TradeRoom = m.Channel
			}
		case <-cancel:
			// TODO: what happens if the player accepts after timeout?
			return nil
		}
	}
	return s.chTradeStatus
}

func (s *State) Trade(tradePartner Player) (ts TradeStatus) {
	// Send them a trade invite and see if they accept
	chTradeStatus := s.InitiateTrade(tradePartner, 40*time.Second)
	if chTradeStatus != nil {
		defer s.LeaveRoom(TradeRoom)
		lastActivity := time.Now()
		startTime := time.Now()

		donation := false

		minuteWarning := false
		tenSecondWarning := false
		lastIdleWarning := time.Now()

		cardsChanged := false

		s.Say(TradeRoom, fmt.Sprintf("Welcome %s. This is an automated trading unit. If you don't know what to do, just say '!help'.", tradePartner))

		request := WTBrequests[tradePartner]
		if len(request) > 0 {
			cardIds := make([]CardUid, 0)

			for cardName, num := range request {
				for _, card := range Libraries[Bot].Cards {
					if card.Tradable && CardTypes[card.TypeId] == cardName {
						cardIds = append(cardIds, card.Id)
						num--
						if num <= 0 {
							break
						}
					}
				}
			}
			s.SendRequest(Request{"msg": "TradeAddCards", "cardIds": cardIds})
			s.Say(TradeRoom, "I've initialized the trade room with your last WTB request. You can !reset to undo this.")
		}

		messages := s.Listen()
		defer s.Shut(messages)
		ticker := time.Tick(time.Second)
		for {
			select {
			case <-s.chQuit:
				log.Printf("Trade quit!")
				s.chQuit <- true
				return
			case m := <-messages:
				if m.From == tradePartner && m.Channel == TradeRoom {
					lastActivity = time.Now()
					command := strings.ToLower(m.Text)

					if command == "!help" {
						s.Say(TradeRoom, "Just add the scrolls you want to sell on your side. To buy scrolls from me, say 'wtb [list of scrolls]'"+
							" and I'll add everything I have on that list. You can also !add or !remove single cards."+
							" Not sure about the gold? Just ask for the !price and I'll list it up.")

					} else if command == "!donation" {
						donation = !donation
						if donation {
							s.Say(TradeRoom, "I will consider everything you put into this trade as a donation. Much appreciated!"+
								" If you change your mind, just repeat the command.")
						} else {
							s.Say(TradeRoom, "Okay :(")
						}

					} else if command == "!reset" {
						for cardName, num := range ts.My.Cards {
							for _, card := range Libraries[Bot].Cards {
								if CardTypes[card.TypeId] == cardName && card.Tradable {
									s.SendRequest(Request{"msg": "TradeRemoveCard", "cardId": card.Id})
									num--
									if num <= 0 {
										break
									}
								}
							}
						}

					} else if command == "!price" {
						format := func(card Card, num int) string {
							if num > 1 {
								return fmt.Sprintf("%dx %s", num, card)
							}
							return string(card)
						}

						theirValue := make(map[string]int)
						for card, num := range ts.Their.Cards {
							theirValue[format(card, num)] = s.DeterminePrice(card, num, true)
						}
						myValue := make(map[string]int)
						for card, num := range ts.My.Cards {
							myValue[format(card, num)] = s.DeterminePrice(card, num, false)
						}

						list := func(value map[string]int) []string {
							lines := make([]string, len(value))
							for i, _ := range lines {
								mostGold := 0
								nextCard := ""

								for card, gold := range value {
									if gold > mostGold {
										mostGold = gold
										nextCard = card
									}
								}
								lines[i] = fmt.Sprintf("%s for %dg", nextCard, mostGold)
								value[nextCard] = 0
							}
							return lines
						}
						msg := ""
						if len(theirValue) > 0 {
							msg += fmt.Sprintf("I'll buy %s. ", strings.Join(list(theirValue), ", "))
						}
						if len(myValue) > 0 {
							msg += fmt.Sprintf("I'll sell %s. ", strings.Join(list(myValue), ", "))
						}
						diff := ts.Their.Value - ts.My.Value
						if diff < 0 {
							msg += fmt.Sprintf("Thus you owe me %dg.", -diff)
						} else {
							msg += fmt.Sprintf("Thus I owe you %dg.", diff)
						}
						s.Say(m.Channel, msg)

					} else if strings.HasPrefix(command, "!add") || strings.HasPrefix(command, "!wtb") || strings.HasPrefix(command, "wtb") {
						cardlist := strings.TrimPrefix(command, "!add")
						cardlist = strings.TrimPrefix(cardlist, "!wtb")
						cardlist = strings.TrimPrefix(cardlist, "wtb")

						cardIds := make([]CardUid, 0)

						requestedCards, ambiguousWords, failedWords := parseCardList(cardlist)

						WTBrequests[tradePartner] = requestedCards
						if len(requestedCards) > 0 {
							missing := make(map[Card]int)
							for requestedCard, num := range requestedCards {
								skip := ts.My.Cards[requestedCard]
								for _, card := range Libraries[Bot].Cards {
									if CardTypes[card.TypeId] != requestedCard || !card.Tradable {
										continue
									}
									skip--
									if num > 0 && skip < 0 {
										cardIds = append(cardIds, card.Id)
										num--
									}
								}
								if num > 0 {
									missing[requestedCard] = num
								}
							}

							reply := ""
							if len(missing) > 0 {
								list := make([]string, 0, len(missing))
								for card, num := range missing {
									list = append(list, fmt.Sprintf("%dx %s", num, card))
								}
								reply = fmt.Sprintf("I don't have %s. ", strings.Join(list, ", "))
							}
							for _, word := range ambiguousWords {
								reply += fmt.Sprintf("'%s' is %s. ", word, orify(matchCardName(word)))
							}
							if len(failedWords) > 0 {
								reply += fmt.Sprintf("I don't know what '%s' is. ", cardlist)
							}
							if reply != "" {
								s.Say(TradeRoom, reply)
							}
							if len(cardIds) > 0 {
								s.SendRequest(Request{"msg": "TradeAddCards", "cardIds": cardIds})
							}
						}

					} else if command == "!remove" {
						s.Say(TradeRoom, "You have to name the card that I will remove.")

					} else if strings.HasPrefix(command, "!remove") {
						params := strings.TrimPrefix(command, "!remove ")
						matchedCards := matchCardName(params)
						switch len(matchedCards) {
						case 0:
							s.Say(m.Channel, fmt.Sprintf("There is no scroll named '%s'.", params))
						case 1:
							card := matchedCards[0]
							alreadyOffered := ts.My.Cards[card]
							if alreadyOffered == 0 {
								s.Say(m.Channel, fmt.Sprintf("%s is not part of this trade!", card))
							} else {
								for _, mcard := range Libraries[Bot].Cards {
									if mcard.Tradable && CardTypes[mcard.TypeId] == card {
										if alreadyOffered == 1 {
											s.SendRequest(Request{"msg": "TradeRemoveCard", "cardId": mcard.Id})
											break
										}
										alreadyOffered--
									}
								}
							}
						default:
							s.Say(m.Channel, fmt.Sprintf("'%s' is %s.", params, matchCardName(params)))
						}
					}
				}

				if m.From == "Scrolls" && m.Channel == TradeRoom && strings.HasPrefix(m.Text, "Trade ended") {
					return
				}

			case newTradeStatus := <-chTradeStatus:
				oldValueSum := ts.Their.Value + ts.My.Value

				ts = newTradeStatus
				if ts.Updated {
					lastActivity = time.Now()
				}

				if ts.My.Accepted && ts.Their.Accepted {
					s.Say(TradeRoom, "Thanks!")
					if donation {
						if diff := ts.Their.Value + ts.Their.Gold - ts.My.Value - ts.My.Gold; diff > 0 {
							s.Say(Conf.Room, fmt.Sprintf("%s just donated stuff worth %dg. Praise to them!", tradePartner, diff))
						}
					}

					Gold += ts.Their.Gold
					Gold -= ts.My.Gold
					for card, num := range ts.Their.Cards {
						Stocks[Bot][card] += num
					}
					for card, num := range ts.My.Cards {
						Stocks[Bot][card] -= num
					}

					alreadySold := make(map[Card]bool)
					cardIds := make([]CardUid, 0)

					for _, card := range Libraries[Bot].Cards {
						cardName := CardTypes[card.TypeId]
						if !alreadySold[cardName] && card.Tradable && s.DeterminePrice(cardName, 1, true) <= MinimumValue(cardName) {
							alreadySold[cardName] = true
							cardIds = append(cardIds, card.Id)
						}
					}
					if len(cardIds) > 0 {
						s.SendRequest(Request{"msg": "SellCards", "cardIds": cardIds})
						for _, id := range cardIds {
							for _, card := range Libraries[Bot].Cards {
								if card.Id == id {
									name := CardTypes[card.TypeId]
									Stocks[Bot][name] = Stocks[Bot][name] - 1
									Gold += MinimumValue(name)
									break
								}
							}
						}
					}
					WTBrequests[tradePartner] = nil
					logTrade(ts)
					return
				}

				for card, num := range ts.Their.Cards {
					ts.Their.Value += s.DeterminePrice(card, num, true)
				}

				for card, num := range ts.My.Cards {
					ts.My.Value += s.DeterminePrice(card, num, false)
				}

				if oldValueSum != ts.Their.Value+ts.My.Value {
					cardsChanged = true
				}

				goldNeeded := ts.Their.Value - ts.My.Value + ts.Their.Gold
				if goldNeeded != ts.My.Gold {
					if goldNeeded > 0 && GoldForTrade() >= goldNeeded {
						s.SendRequest(Request{"msg": "TradeSetGold", "gold": goldNeeded})
					} else if ts.My.Gold != 0 {
						s.SendRequest(Request{"msg": "TradeSetGold", "gold": 0})
					}
				}

			case <-ticker:
				if time.Now().After(lastActivity.Add(time.Minute)) && time.Now().After(lastIdleWarning.Add(time.Minute)) {
					s.Say(TradeRoom, "You have been idle for a minute. This trade window will close in 30 seconds unless you interact with it.")
					lastIdleWarning = time.Now()
				}

				if time.Now().After(lastActivity.Add(time.Minute + 30*time.Second)) {
					s.Say(TradeRoom, "Time's up!")
					return
				}

				if !minuteWarning && time.Now().After(startTime.Add(4*time.Minute)) {
					s.Say(TradeRoom, "Please finish the trade within the next minute.")
					minuteWarning = true
				}
				if !tenSecondWarning && time.Now().After(startTime.Add(4*time.Minute+50*time.Second)) {
					s.Say(TradeRoom, "You have 10 seconds left to finish the trade.")
					tenSecondWarning = true
				}
				if time.Now().After(startTime.Add(time.Minute * 5)) {
					s.Say(TradeRoom, "Time's up!")
					return
				}

				if cardsChanged && time.Now().After(lastActivity.Add(time.Second*2)) {
					cardsChanged = false

					value := ts.Their.Value - ts.My.Value
					if value > GoldForTrade() && !donation {
						s.Say(TradeRoom, fmt.Sprintf("Sorry - I only have %d gold at my disposal. Please take something out. Or is this a !donation?", GoldForTrade()))
					} else if value < 0 {
						s.Say(TradeRoom, fmt.Sprintf("Please set your gold offer to %dg", -value))
					}
				}

				myGain := ts.Their.Value + ts.Their.Gold
				theirGain := ts.My.Value + ts.My.Gold
				canAccept := false

				if myGain >= theirGain && myGain > 0 {
					canAccept = true
					if !donation {
						canAccept = myGain == theirGain
					}
				}

				// s.Say(TradeRoom, fmt.Sprintf("%d %d %s %s", myGain, theirGain, canAccept, donation))

				if canAccept && !ts.My.Accepted && time.Now().After(lastActivity.Add(time.Second*7)) {
					s.SendRequest(Request{"msg": "TradeAcceptBargain"})
				}
			}
		}
	}
	return
}

func logTrade(ts TradeStatus) {
	file, err := os.OpenFile("trade.log", os.O_WRONLY+os.O_CREATE+os.O_APPEND, 0)
	deny(err)
	defer file.Close()

	list := func(count map[Card]int) string {
		s := make([]string, 0)
		for card, num := range count {
			if num > 1 {
				s = append(s, fmt.Sprintf("%dx %s", num, card))
			} else {
				s = append(s, string(card))
			}
		}
		return strings.Join(s, ",")
	}

	io.WriteString(file, fmt.Sprintf("%s: Traded with %s.\nTheir offer: [%dg] %s\nMy offer: [%dg] %s\n\n",
		time.Now().String(), ts.Partner, ts.Their.Gold, list(ts.Their.Cards), ts.My.Gold, list(ts.My.Cards)))
}
