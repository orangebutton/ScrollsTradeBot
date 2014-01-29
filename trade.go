package main

import (
	"fmt"
	"math"
	"strings"
	"time"
)

func (s *State) Trade(player Player) (currentTradeStatus TradeStatus) {
	tradeStatus := s.InitiateTrade(player, 30*time.Second)
	if tradeStatus == nil {
	} else {
		defer s.LeaveRoom(s.CurrentTradeRoom)

		alreadyAdvertised := make(map[string]struct {
			Buy  bool
			Sell bool
		})
		ticker := time.Tick(time.Second)

		canAccept := false
		messages := s.Listen()

		lastActivity := time.Now()

		startTime := time.Now()
		minuteWarning := false
		tenSecondWarning := false
		cardsChanged := false
		lastIdleWarning := time.Now()
		myValue := 0
		theirValue := 0
		maxGoldForThisTrade := s.Gold / 5

		s.Say(s.CurrentTradeRoom, fmt.Sprintf("Welcome %s. This is an automated trading unit. If you don't know what to do, just say '!help'.", s.CurrentTradePartner))

		for {
			select {
			case m := <-messages:
				if m.From == s.CurrentTradePartner && m.Channel == s.CurrentTradeRoom {
					lastActivity = time.Now()

					if m.Text == "!help" {
						s.Say(m.Channel, "When you add and remove scrolls to the trade, I will automatically update my gold offer.")
						s.Say(m.Channel, "To buy scrolls from me, say '!add [scroll name]' or '!remove [scroll name]' and I will update my offer.")
					} else if strings.HasPrefix(m.Text, "!add") {
						cardName := matchCardName(strings.Replace(m.Text, "!add ", "", 1))
						stocked, ok := Stocks[s.Player][cardName]
						alreadyOffered := 0
						for _, card := range currentTradeStatus.My.Cards {
							if card == cardName {
								alreadyOffered++
							}
						}

						if !ok {
							s.Say(m.Channel, fmt.Sprintf("There is no scroll named '%s'.", cardName))
						} else if alreadyOffered == stocked {
							s.Say(m.Channel, fmt.Sprintf("Sorry, %s is out of stock.", cardName))
						} else {
							for _, card := range Libraries[s.Player].Cards {
								if card.Tradable && CardTypes[CardId(card.TypeId)] == cardName {
									if alreadyOffered == 0 {
										s.SendRequest(Request{"msg": "TradeAddCards", "cardIds": []int{card.Id}})
										break
									}
									alreadyOffered--
								}
							}
						}
					} else if strings.HasPrefix(m.Text, "!remove") {
						cardName := matchCardName(strings.Replace(m.Text, "!remove ", "", 1))
						_, ok := Stocks[s.Player][cardName]

						alreadyOffered := 0
						for _, card := range currentTradeStatus.My.Cards {
							if card == cardName {
								alreadyOffered++
							}
						}

						if !ok {
							s.Say(m.Channel, fmt.Sprintf("There is no scroll named '%s'.", cardName))
						} else if alreadyOffered == 0 {
							s.Say(m.Channel, fmt.Sprintf("%s is not part of this trade!", cardName))
						} else {
							for _, card := range Libraries[s.Player].Cards {
								if card.Tradable && CardTypes[CardId(card.TypeId)] == cardName {
									if alreadyOffered == 1 {
										s.SendRequest(Request{"msg": "TradeRemoveCard", "cardId": card.Id})
										break
									}
									alreadyOffered--
								}
							}
						}
					}
				}

				if m.From == "Scrolls" && m.Channel == s.CurrentTradeRoom && strings.HasPrefix(m.Text, "Trade ended") {
					return
				}
			case ts := <-tradeStatus:
				theirValue = 0
				myValue = 0
				cardsChanged = false
				if len(currentTradeStatus.My.Cards) != len(ts.My.Cards) ||
					len(currentTradeStatus.Their.Cards) != len(ts.Their.Cards) {
					cardsChanged = true
				}

				if ts.Updated {
					lastActivity = time.Now()
				}

				currentTradeStatus = ts
				if ts.My.Accepted && ts.Their.Accepted {
					s.Say(s.CurrentTradeRoom, "Thanks!")
					s.Gold += ts.Their.Gold
					s.Gold -= ts.My.Gold
					logTrade(currentTradeStatus)
					return
				}

				for _, card := range ts.Their.Cards {
					price := Prices[card].Buy
					stocked := Stocks[s.Player][card]
					if stocked > 3 {
						price = int(float64(price) * math.Pow(0.80, float64(stocked-3)))
					}

					aa := alreadyAdvertised[card]
					if !aa.Buy {
						s.Say(s.CurrentTradeRoom, fmt.Sprintf("Buying %s for %dg", card, price))
						aa.Buy = true
						alreadyAdvertised[card] = aa
					}
					theirValue += price
				}

				for _, card := range ts.My.Cards {
					price := Prices[card].Sell
					stocked := Stocks[s.Player][card]
					if stocked < 4 {
						price = int(float64(price) * math.Pow(1.05, float64(4-stocked)))
					}

					aa := alreadyAdvertised[card]
					if !aa.Sell {
						s.Say(s.CurrentTradeRoom, fmt.Sprintf("Selling %s for %dg", card, price))
						aa.Sell = true
						alreadyAdvertised[card] = aa
					}
					myValue += price
				}

				theirValue += ts.Their.Gold
				myValue += ts.My.Gold

				canAccept = !ts.My.Accepted && myValue == theirValue && myValue > 0
				if ts.Partner == "redefiance" && ts.Their.Accepted {
					canAccept = true
				}

			case <-ticker:
				if time.Now().After(lastActivity.Add(time.Minute)) && time.Now().After(lastIdleWarning.Add(time.Minute)) {
					s.Say(s.CurrentTradeRoom, "You have been idle for a minute. This trade window will close in 30 seconds unless you interact with it.")
					lastIdleWarning = time.Now()
				}

				if time.Now().After(lastActivity.Add(time.Minute + 30*time.Second)) {
					s.Say(s.CurrentTradeRoom, "Time's up!")
					return
				}

				if !minuteWarning && time.Now().After(startTime.Add(4*time.Minute)) {
					s.Say(s.CurrentTradeRoom, "Please finish the trade within the next minute.")
					minuteWarning = true
				}
				if !tenSecondWarning && time.Now().After(startTime.Add(4*time.Minute+50*time.Second)) {
					s.Say(s.CurrentTradeRoom, "You have 10 seconds left to finish the trade.")
					tenSecondWarning = true
				}
				if time.Now().After(startTime.Add(time.Minute * 5)) {
					s.Say(s.CurrentTradeRoom, "Time's up!")
					return
				}

				if cardsChanged && time.Now().After(lastActivity.Add(time.Second*2)) {
					if currentTradeStatus.My.Gold > theirValue {
						s.SendRequest(Request{"msg": "TradeSetGold", "gold": 0})
					} else {
						diff := theirValue - myValue
						myGoldOffer := diff + currentTradeStatus.My.Gold
						if myGoldOffer > maxGoldForThisTrade {
							s.Say(s.CurrentTradeRoom, fmt.Sprintf("Sorry - your offer is worth %dg, but I currently only have %dg at my disposal",
								theirValue-myValue+currentTradeStatus.My.Gold, maxGoldForThisTrade))
						} else if myGoldOffer < 0 {
							if currentTradeStatus.My.Gold > 0 {
								s.SendRequest(Request{"msg": "TradeSetGold", "gold": 0})
							}
							s.Say(s.CurrentTradeRoom, fmt.Sprintf("Please set your gold offer to %dg", -myGoldOffer))
						} else {
							s.SendRequest(Request{"msg": "TradeSetGold", "gold": myGoldOffer})
						}
					}
					cardsChanged = false
				}

				if canAccept && time.Now().After(lastActivity.Add(time.Second*7)) {
					s.SendRequest(Request{"msg": "TradeAcceptBargain"})
				}
			}
		}
	}
	return
}
