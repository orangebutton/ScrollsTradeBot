package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

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

func (s *State) Trade(tradePartner Player) {
	s.Say(Conf.Room, fmt.Sprintf("Now trading with [%s].", tradePartner))

	// Send them a trade invite and see if they accept
	chTradeStatus := s.initiateTrade(tradePartner)
	if chTradeStatus == nil {
		return
	}

	var ts TradeStatus

	// lets stay in the trade room so that they can send us messages
	// defer s.LeaveRoom(TradeRoom)
	lastActivity := time.Now()
	startTime := time.Now()

	minuteWarning := false
	tenSecondWarning := false
	lastIdleWarning := time.Now()

	cardsChanged := false
	donation := false

	stockBefore := Stocks[Bot]
	if stockBefore == nil {
		stockBefore = make(map[Card]int)
	}

	s.Say(TradeRoom, fmt.Sprintf("Welcome %s. This is an automated trading unit of the strategic angels guild. "+tradeHelpText, tradePartner))
	s.initFromOldWTBRequest(tradePartner)

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
			if m.From == "Scrolls" && m.Channel == TradeRoom && strings.HasPrefix(m.Text, "Trade ended") {
				return
			}
			if m.From == tradePartner && m.Channel == TradeRoom {
				lastActivity = time.Now()
				donation = s.TradeMessageHandler(donation, m, tradePartner, ts)
			}

		case newTradeStatus := <-chTradeStatus:
			oldValueSum := ts.Their.Value + ts.My.Value

			ts = newTradeStatus
			if ts.Updated {
				lastActivity = time.Now()
			}

			if ts.My.Accepted && ts.Their.Accepted {
				s.finishTrade(donation, tradePartner, ts)
				s.acquiredOrSoldMessage(stockBefore, ts)
				//sellExcessInventoryToStore()

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
				s.sayGoldOwed(ts, donation)
			}
			canAccept := isFairTrade(donation, ts)

			// s.Say(TradeRoom, fmt.Sprintf("%d %d %s %s", myGain, theirGain, canAccept, donation))
			if canAccept && !ts.My.Accepted && time.Now().After(lastActivity.Add(time.Second*7)) {
				s.SendRequest(Request{"msg": "TradeAcceptBargain"})
			}
		}
	}
}

func (s *State) initiateTrade(player Player) chan TradeStatus {

	s.SendRequest(Request{"msg": "TradeInvite", "profile": PlayerIds[player]})
	accepted := false
	TradeRoom = ""

	timeout := 65 * time.Second // the scrolls timeout is about 60 seconds, but add 5 seconds for lags
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

func (s *State) initFromOldWTBRequest(tradePartner Player) {
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
}

func (s *State) finishTrade(donation bool, tradePartner Player, ts TradeStatus) {
	s.Say(TradeRoom, "Thanks!")
	if donation {
		if diff := ts.Their.Value + ts.Their.Gold - ts.My.Value - ts.My.Gold; diff > 0 {
			s.Say(Conf.Room, fmt.Sprintf("%s just donated stuff worth %dg. Praise to them!", tradePartner, diff))
		}
	}

	updateInventory(ts)

	WTBrequests[tradePartner] = nil
	logTrade(ts)
}

func updateInventory(ts TradeStatus) {
	Gold += ts.Their.Gold
	Gold -= ts.My.Gold
	for card, num := range ts.Their.Cards {
		Stocks[Bot][card] += num
	}
	for card, num := range ts.My.Cards {
		Stocks[Bot][card] -= num
	}
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

func (s *State) sellExcessInventoryToStore() {
	cardIds := make([]CardUid, 0)
	alreadySold := make(map[Card]bool)
	for _, card := range Libraries[Bot].Cards {
		cardName := CardTypes[card.TypeId]
		if !alreadySold[cardName] && card.Tradable &&
			// prices can never be less than store prices now
			s.DeterminePrice(cardName, 1, false) <= StoreValue(cardName) {

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
					Gold += StoreValue(name)
					break
				}
			}
		}
	}
}

func (s *State) sayGoldOwed(ts TradeStatus, donation bool) {
	value := ts.Their.Value - ts.My.Value
	if value > GoldForTrade() && !donation {
		s.Say(TradeRoom, fmt.Sprintf("Sorry - I only have %d gold at my disposal. Please take something out. Or is this a !donation?", GoldForTrade()))
	} else if value < 0 {
		s.Say(TradeRoom, fmt.Sprintf("Please set your gold offer to %dg", -value))
	}
}

func isFairTrade(donation bool, ts TradeStatus) bool {
	myGain := ts.Their.Value + ts.Their.Gold
	theirGain := ts.My.Value + ts.My.Gold
	canAccept := false

	if myGain >= theirGain && myGain > 0 {
		canAccept = true
		if !donation {
			canAccept = (myGain == theirGain)
		}
	}
	return canAccept
}

func (s *State) acquiredOrSoldMessage(stockBefore map[Card]int, ts TradeStatus) {
	aquired := make([]Card, 0)
	lost := make([]Card, 0)
	for card, num := range ts.Their.Cards {
		if stockBefore[card] == 0 {
			aquired = append(aquired, card)
		}
		stockBefore[card] = stockBefore[card] + num
	}
	for card, num := range ts.My.Cards {
		if stockBefore[card] <= num {
			lost = append(lost, card)
		}
		stockBefore[card] = stockBefore[card] - num
	}
	if len(aquired) > 0 {
		s.Say(Conf.Room, fmt.Sprintf("I've just acquired %s.", andify(aquired)))
	}
	if len(lost) > 0 {
		s.Say(Conf.Room, fmt.Sprintf("I've just sold my last %s.", andify(lost)))
	}
}

func logTrade(ts TradeStatus) {
	file, err := os.OpenFile("Applications/StrategicBot/trade.log", os.O_WRONLY+os.O_CREATE+os.O_APPEND, 0)
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
