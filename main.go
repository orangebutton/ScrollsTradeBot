package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	MyRoom = "clockwork" // please change this when you set up your own bot :)
)

var reNumbers = regexp.MustCompile(`x?(\d+)x?`)
var reInvalidChars = regexp.MustCompile("[^a-z'0-9 ]")

var WTBrequests = make(map[Player]map[Card]int)
var Bot Player

func main() {

	// Logging..
	//f, err := os.OpenFile("system.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	//deny(err)
	//log.SetOutput(f)
	log.SetOutput(ioutil.Discard)

	// Get email and password from the login.txt file (2 lines)
	login, err := ioutil.ReadFile("login.txt")
	deny(err)

	lines := strings.Split(string(login), "\n")
	if len(lines) != 2 { // try windows line endings
		lines = strings.Split(string(login), "\r\n")
	}
	if len(lines) != 2 {
		panic("could not read email/password from login.txt")
	}

	email, password := lines[0], lines[1]
	startBot(email, password, "Hello world!")
	for {
		startBot(email, password, "I live again!")
	}
}

func deny(err error) {
	if err != nil {
		panic(err)
	}
}

func startBot(email, password, helloMessage string) {
	log.Print("Connecting...")
	s, chAlive := Connect(email, password)

	s.JoinRoom(MyRoom)
	if helloMessage != "" {
		s.Say(MyRoom, helloMessage)
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
				if m.From == "redefiance" && strings.HasPrefix(m.Text, "!say ") {
					s.Say(MyRoom, strings.TrimPrefix(m.Text, "!say "))
				}

				forceWhisper := false
				replyMsg := ""
				command := strings.ToLower(m.Text)

				// if m.From != "redefiance" {
				if m.From == "Great_Marcoosai" {
					command = "" // banned!

				}

				if strings.HasPrefix(command, "wt") {
					command = strings.Replace(command, "wt", "!wt", 1)
				}

				if m.Channel == "WHISPER" && !strings.HasPrefix(command, "!") {
					command = "!" + command
				}

				if command == "!wts" || command == "!wtb" {
					replyMsg = "You need to add a list of cards to this command, seperated by commata. Multipliers like '2x' are allowed."
					forceWhisper = true
				}

				if strings.HasPrefix(command, "!wts ") && m.Channel != TradeRoom {
					cards, ambiguousWords, failedWords := parseCardList(strings.TrimPrefix(command, "!wts "))
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

						s2, s3 := "", ""
						if goldSum > GoldForTrade() {
							s3 = fmt.Sprintf(" I currently only have %dg.", GoldForTrade())
						}
						if len(words) > 1 {
							s2 = fmt.Sprintf(" That sums up to %dg.", goldSum)
						}

						replyMsg = fmt.Sprintf("I'm buying %s.%s%s", strings.Join(words, ", "), s2, s3)
						forceWhisper = true
					}

					if len(ambiguousWords) > 0 {
						for _, word := range ambiguousWords {
							replyMsg += fmt.Sprintf(" '%s' is %s.", word, orify(matchCardName(word)))
						}
					}

					if len(failedWords) > 0 {
						replyMsg += fmt.Sprintf(" I don't know what '%s' is.", strings.Join(failedWords, ", "))
					}
				}

				if strings.HasPrefix(command, "!wtb ") && m.Channel != TradeRoom {
					cards, ambiguousWords, failedWords := parseCardList(strings.TrimPrefix(command, "!wtb "))
					WTBrequests[m.From] = cards
					if len(cards) > 0 {
						words := make([]string, 0, len(cards))
						numItems := 0
						goldSum := 0
						hasAll := true
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
						forceWhisper = true
					}
				}

				if strings.HasPrefix(command, "!price ") || strings.HasPrefix(command, "!stock ") {
					params := strings.TrimPrefix(strings.TrimPrefix(command, "!stock "), "!price ")
					matchedCards := matchCardName(params)
					switch len(matchedCards) {
					case 0:
						replyMsg = fmt.Sprintf("There is no card named '%s'", params)
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
				}

				if command == "!missing" {
					list := make([]Card, 0)
					for _, card := range CardTypes {
						if Stocks[Bot][card] == 0 {
							list = append(list, card)
						}
					}
					replyMsg = fmt.Sprintf("I currently don't have %s. I'm paying extra for that!", andify(list))
					forceWhisper = true
				}

				if command == "!stock" {
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

					replyMsg = fmt.Sprintf("I have %d commons, %d uncommons and %d rares. That's %d%% of all card types, as well as %d gold. Total value is %dk gold.",
						commons, uncommons, rares, 100*len(uniques)/len(CardTypes), GoldForTrade(), int(totalValue/1000))
				}

				if command == "!help" && m.Channel != TradeRoom {
					replyMsg = "You can whisper me WTS or WTB requests. If you're interested in trading, you can queue up with '!trade'. You can also check the '!stock'"
				}

				if command == "!uptime" {
					replyMsg = fmt.Sprintf("Up since %s", time.Since(upSince))
				}

				if command == "!trade" || command == "!queue" {
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
				}

				if replyMsg != "" {
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
