package main

import (
	"fmt"
	"strings"
)

const helpText string = "You can whisper me 'wtb' or 'wts' requests. " +
	"Those commands can also use lists of cards separated by commas. " +
	"When you are ready you can 'trade' with me. " +
	"You can also check the 'stock' and what is 'missing'. " +
	"Prices are based on inventory. To get good deals on cards check my prices often. "

func (s *State) HandleMessages(m Message, queue chan<- Player) {

	// the trade handler has its own message handler
	// ignore chat messages from trading-x
	if m.From == "Scrolls" ||
		m.From == Bot ||
		m.From == "Great_Marcoosai" || // banned
		m.Channel == TradeRoom ||
		strings.HasPrefix(string(m.Channel), "trading-") {
		return
	}

	forceWhisper := false
	replyMsg := ""
	command, args := ParseCommandAndArgs(m.Text)

	switch command {
	case "!help":
		replyMsg = helpText
		forceWhisper = (m.Channel == TradeRoom)
	case "!stock":
		replyMsg = s.handleStock()
	case "!wtb":
		replyMsg = s.handleWTB(args, m.From)
		forceWhisper = (m.Channel == TradeRoom)
	case "!wts":
		replyMsg = s.handleWTS(args)
		forceWhisper = (m.Channel == TradeRoom)
	case "!price":
		replyMsg = s.handlePrice(args)
		forceWhisper = (m.Channel == TradeRoom)
	case "!missing":
		replyMsg = handleMissing()
		forceWhisper = (m.Channel == TradeRoom)
	case "!trade", "!queue":
		replyMsg = handleTrade(m, queue)
	default:
		s.handleOwnerCommands(command, args, m.From)
	}

	if replyMsg != "" {
		s.sayReplay(replyMsg, forceWhisper, m)
	}
}

func (s *State) handleOwnerCommands(command, args string, from Player) {

	if from != Conf.Owner {
		return
	}
	switch command {
	case "!say":
		tokens := strings.SplitN(args, " ", 2)
		s.Say(Channel(tokens[0]), tokens[1])
	case "!whisper", "!w":
		tokens := strings.SplitN(args, " ", 2)
		s.Whisper(Player(tokens[0]), tokens[1])
	case "!hello":
		s.Say(Channel("trading-1"), HelloMessage)
		s.Say("trading-2", HelloMessage)
	case "!join":
		s.JoinRoom(Channel(args))
	case "!leave":
		s.LeaveRoom(Channel(args))
	}
	//case "!uptime":
	//	replyMsg = fmt.Sprintf("Up since %s", time.Since(upSince))
}

func ParseCommandAndArgs(text string) (command, args string) {

	text = strings.TrimSpace(strings.ToLower(text))
	strs := strings.SplitN(text, " ", 2)
	command = strings.TrimSpace(strs[0])
	if len(strs) > 1 {
		args = strings.TrimSpace(strs[1])
	}

	if !strings.HasPrefix(text, "!") {
		command = "!" + command
	}

	return command, args
}

func handleMissing() string {
	list := make([]Card, 0)
	for _, card := range CardTypes {
		if Stocks[Bot][card] == 0 {
			list = append(list, card)
		}
	}
	return fmt.Sprintf("I currently don't have %s. I'm paying extra for that!", andify(list))
}

func (s *State) handleStock() string {
	commons := 0
	uncommons := 0
	rares := 0
	uniques := make(map[Card]bool)
	totalValue := 0

	for _, card := range Libraries[Bot].Cards {
		name := CardTypes[card.TypeId]
		if uniques[name] == false {
			totalValue += s.DeterminePrice(name, Stocks[Bot][name], false)
		}
		uniques[name] = true
		switch CardRarities[name] {
		case 0:
			commons++
		case 1:
			uncommons++
		case 2:
			rares++
		}
	}

	totalValue += Gold

	return fmt.Sprintf("I have %d commons, %d uncommons and %d rares. That's %d%% of all card types, as well as %d gold. Total value is %dk gold. ", commons, uncommons, rares, 100*len(uniques)/len(CardTypes), GoldForTrade(), int(totalValue/1000))
}

func (s *State) handleWTB(args string, from Player) (replyMsg string) {

	if args == "" {
		return "You need to add a list of cards to this command, seperated by comma. Multipliers like '2x' are allowed."
	}

	cards, ambiguousWords, failedWords := parseCardList(args)
	WTBrequests[from] = cards
	words := make([]string, 0, len(cards))
	goldSum := 0
	hasAll := true
	numItems := 0

	for card, num := range cards {
		forceNumStr := false
		numItems += num
		if stocked := Stocks[Bot][card]; num > stocked {
			num = stocked
			hasAll = false
			forceNumStr = true
			if num == 0 {
				continue
			}
		}

		gold := s.DeterminePrice(card, num, false)
		numStr := ""
		if forceNumStr || num != 1 {
			numStr = fmt.Sprintf("%dx ", num)
		}
		words = append(words, fmt.Sprintf("%s%s %d", numStr, card, gold))
		goldSum += gold
	}

	s1, s2, s3 := "", "", ""
	if !hasAll {
		s1 = " That's all I have."
	}
	if len(words) > 1 {
		s2 = fmt.Sprintf(" That sums up to %dg.", goldSum)
	}
	if len(failedWords) > 0 {
		s3 = fmt.Sprintf(" I don't know what '%s' is.", strings.Join(failedWords, ", "))
	}

	if len(ambiguousWords) > 0 {
		for _, word := range ambiguousWords {
			s3 = fmt.Sprintf(" '%s' is %s.", word, orify(matchCardName(word))) + s3
		}
	}

	if goldSum == 0 {
		if numItems == 1 {
			replyMsg = "I don't have "
			for card, _ := range cards {
				replyMsg += string(card)
				break
			}
			replyMsg += " stocked."
		} else {
			replyMsg = "I don't have anything on that list stocked."
		}
		replyMsg += s3
	} else {
		replyMsg = fmt.Sprintf("I'm selling %s.%s%s%s", strings.Join(words, ", "), s1, s2, s3)
	}
	return
}

func (s *State) handleWTS(args string) (replyMsg string) {
	if args == "" {
		return "You need to add a list of cards to this command, seperated by comma. Multipliers like '2x' are allowed."
	}

	cards, ambiguousWords, failedWords := parseCardList(args)
	if len(cards) > 0 {
		words := make([]string, 0, len(cards))
		goldSum := 0
		for card, num := range cards {
			gold := s.DeterminePrice(card, num, true)
			numStr := ""
			if num != 1 {
				numStr = fmt.Sprintf("%dx ", num)
			}
			words = append(words, fmt.Sprintf("%s%s %d", numStr, card, gold))
			goldSum += gold
		}

		var s2 = ""
		var s3 = ""
		//		s2, s3 := "", ""
		if goldSum > GoldForTrade() {
			s3 = fmt.Sprintf(" I currently only have %dg.", GoldForTrade())
		}
		if len(words) > 1 {
			s2 = fmt.Sprintf(" That sums up to %dg.", goldSum)
		}

		replyMsg = fmt.Sprintf("I'm buying %s.%s%s", strings.Join(words, ", "), s2, s3)
	}

	if len(ambiguousWords) > 0 {
		for _, word := range ambiguousWords {
			replyMsg += fmt.Sprintf(" '%s' is %s.", word, orify(matchCardName(word)))
		}
	}

	if len(failedWords) > 0 {
		replyMsg += fmt.Sprintf(" I don't know what '%s' is.", strings.Join(failedWords, ", "))
	}

	return
}

func (s *State) handlePrice(args string) (replyMsg string) {
	matchedCards := matchCardName(args)

	if len(matchedCards) == 0 {
		replyMsg = fmt.Sprintf("There is no card named %s", args)
	} else {
		for i, card := range matchedCards {
			if i > 0 {
				replyMsg += "\n"
			}

			stocked := Stocks[Bot][card]
			if stocked == 0 {
				price := s.DeterminePrice(card, 1, true)
				replyMsg += string(card) + " is out of stock. "
				if price > GoldForTrade() {
					replyMsg += fmt.Sprintf("I would buy %s for %d, but I don't have that much. ", card, price)
				} else {
					replyMsg += fmt.Sprintf("I'm buying %s for %d.", card, price)
				}
			} else {
				replyMsg += fmt.Sprintf("I'm buying %s for %d and selling for %d (%d stocked).", card,
					s.DeterminePrice(card, 1, true), s.DeterminePrice(card, 1, false), stocked)
			}
		}
	}
	return
}

func handleTrade(m Message, queue chan<- Player) string {

	//if inQueue(m.From, queue) {
	//	return fmt.Sprintf("You are already queued for trading.")
	//}

	replyMsg := ""

	queue <- m.From
	if len(queue) > 1 {
		if m.Channel != "WHISPER" {
			replyMsg = fmt.Sprintf("%s: ", m.From)
		}
		replyMsg += fmt.Sprintf("You are now queued for trading. Your position in the queue is %d.", len(queue)-1)
	}

	return replyMsg
}

func (s *State) sayReplay(replyMsg string, forceWhisper bool, m Message) {
	if m.Channel == "WHISPER" {
		s.Whisper(m.From, replyMsg)
	} else {
		if forceWhisper {
			s.Whisper(m.From, replyMsg)
			s.Whisper(m.From, "To avoid spamming the channel, please use this command only in whisper. "+
				"By the way, you can use any other command in whisper as well (even without the ! in the front).")
		} else {
			s.Say(m.Channel, replyMsg)
		}
	}
}
