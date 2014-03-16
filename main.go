package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"
)

var reNumbers = regexp.MustCompile(`x?(\d+)x?`)
var reInvalidChars = regexp.MustCompile("[^a-z'0-9 ]")

var WTBrequests = make(map[Player]map[Card]int)
var Bot Player
var currentState *State

const MyRoom = "autobots" // you will need to change the room

func main() {
	log.Print("main start...")

	config := LoadConfig()

	if config.Log {
		f, err := os.OpenFile("system.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		deny(err)
		log.SetOutput(f)
		//log.SetOutput(ioutil.Discard)
	}

	if config.UseWebserver {
		go startWebServer()
	}

	for {
		startBot(config.Email, config.Password, "Hello world!")
	}
}

func deny(err error) {
	if err != nil {
		panic(err)
	}
}

func startBot(email, password, helloMessage string) {

	s, chAlive := Connect(email, password)
	currentState = s

	s.JoinRoom(MyRoom)
	s.JoinRoom("trading-1")

	if helloMessage != "" {
		s.Say(MyRoom, helloMessage)
		s.Say("trading-1", helloMessage)
	}

	upSince := time.Now()

	chKillThread := make(chan bool, 1)

	go func() {
		queue := make([]Player, 0)

		chReadyToTrade := make(chan bool, 100)
		currentlyTrading := false

		messages := s.Listen()
		defer s.Shut(messages)

		for {
			select {
			case <-chKillThread:
				return

			case <-chReadyToTrade:
				if len(queue) == 0 {
					s.Say(MyRoom, "Finished trading.")
					currentlyTrading = false
				} else {
					currentlyTrading = true

					go func() {
						waiting := make([]string, len(queue)-1)
						for i, name := range queue[1:] {
							waiting[i] = string(name)
						}
						if len(waiting) > 0 {
							s.Say(MyRoom, fmt.Sprintf("Now trading with [%s] < %s", queue[0], strings.Join(waiting, " < ")))
						} else {
							s.Say(MyRoom, fmt.Sprintf("Now trading with [%s].", queue[0]))
						}

						stockBefore := Stocks[Bot]
						if stockBefore == nil {
							stockBefore = make(map[Card]int)
						}

						ts := s.Trade(queue[0])

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
							s.Say(MyRoom, fmt.Sprintf("I've just aquired %s.", andify(aquired)))
						}
						if len(lost) > 0 {
							s.Say(MyRoom, fmt.Sprintf("I've just sold my last %s.", andify(lost)))
						}

						queue = queue[1:]
						chReadyToTrade <- true
					}()
				}

			case m := <-messages:

				forceWhisper := false
				replyMsg := ""
				command, args := getCommand(m)

				// ignore chat messages from trading-1
				if m.Channel != "trading-1" {

					switch command {
					case "!help":
						replyMsg = "You can whisper me WTS or WTB requests. If you're interested in trading, you can queue up with '!trade'. You can also check the '!stock' and what is '!missing.'"
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
					case "say":
						//if m.From == "redefiance" {
						s.Say(MyRoom, args)
					case "sayTrading1":
						s.Say("trading-1", args)
					case "!uptime":
						replyMsg = fmt.Sprintf("Up since %s", time.Since(upSince))
					case "!trade", "!queue":
						for i, player := range queue {
							if player == m.From {
								replyMsg = fmt.Sprintf("You are already queued for trading. Your position in the queue is %d.", i)
								break
							}
						}
						if replyMsg == "" {
							queue = append(queue, m.From)
							if len(queue) == 1 && !currentlyTrading {
								chReadyToTrade <- true
							} else {
								if m.Channel != "WHISPER" {
									replyMsg = fmt.Sprintf("%s: ", m.From)
								}
								replyMsg += fmt.Sprintf("You are now queued for trading. Your position in the queue is %d.", len(queue)-1)
							}
						}
					} // end switch

					if replyMsg != "" {
						s.sayReplay(replyMsg, forceWhisper, m)
					}
				}
			}
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("recovered from: %s", r)
		}

		log.Print("killing thread")
		chKillThread <- true
		log.Print("Restarting..")
	}()

	for {
		timeout := time.After(time.Minute * 1)
	InnerLoop:
		for {
			select {
			case <-chAlive:
				break InnerLoop
			case <-s.chQuit:
				log.Println("!!!main QUIT!!!")
				s.chQuit <- true
				return
			case <-timeout:
				log.Println("!!!TIMEOUT!!!")
				return
			}
		}
	}
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
	switch len(matchedCards) {
	case 0:
		replyMsg = fmt.Sprintf("There is no card named '%s'", args)
	case 1:
		card := matchedCards[0]
		stocked := Stocks[Bot][card]
		if stocked == 0 {
			price := s.DeterminePrice(card, 1, true)
			replyMsg = string(card) + " is out of stock. "
			if price > GoldForTrade() {
				replyMsg += fmt.Sprintf("I would buy for %dg, but I don't have that much.", price)
			} else {
				replyMsg += fmt.Sprintf("I'm buying for %dg.", price)
			}

		} else {
			replyMsg = fmt.Sprintf("I'm buying %s for %dg and selling for %dg (%d stocked).", card,
				s.DeterminePrice(card, 1, true), s.DeterminePrice(card, 1, false), stocked)
		}
	default:
		replyMsg = fmt.Sprintf("Did you mean %s?", orify(matchedCards))
	}

	if rand.Float64() > 0.95 {
		replyMsg += " By the way, you can whisper me with 'wtb/wts [list of cards]' to easily check prices and availability for all cards you're interested in."
	}
	return
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

func getCommand(m Message) (command, args string) {

	if m.From == "Great_Marcoosai" {
		return "", ""
	}

	text := strings.ToLower(m.Text)
	strs := strings.Split(text, " ")
	command = strings.TrimSpace(strs[0])
	if len(strs) > 1 {
		args = strings.TrimSpace(strs[1])
	}

	if !strings.HasPrefix(text, "!") {
		command = "!" + command
	}

	return command, args
}
