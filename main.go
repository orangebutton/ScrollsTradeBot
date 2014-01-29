// TODO: !add [scroll] [quantity]
// TODO: reduce price for multiple scrolls
// TODO: ask for scrolls prices outside trade

package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func main() {
	startBot("I have been renewed!")
}

type Price struct{ Buy, Sell int }

var Prices = make(map[string]Price)

func matchCardName(input string) string {
	minDist := 4
	bestFit := input

	for _, cardName := range CardTypes {
		dist := Levenshtein(strings.ToLower(input), strings.ToLower(cardName))
		if dist < minDist {
			minDist = dist
			bestFit = cardName
		}
	}
	if minDist > 0 {
		for _, cardName := range CardTypes {
			for _, substr := range strings.Split(cardName, " ") {
				dist := Levenshtein(strings.ToLower(input), strings.ToLower(substr))
				if dist < minDist {
					minDist = dist
					bestFit = cardName
				}
			}
		}
	}
	return bestFit
}

func startBot(helloMessage string) {
	LoadPrices()

	state, chAlive := Connect("scrolldier@gmail.com", "my little password")
	state.JoinRoom("clockwork")
	state.Say("clockwork", helloMessage)

	chKillThread := make(chan bool)

	go func() {
		queue := make([]Player, 0)

		chReadyToTrade := make(chan bool, 100)
		currentlyTrading := false

		for {
			select {
			case <-chKillThread:
				return

			case <-chReadyToTrade:
				if len(queue) > 0 {
					currentlyTrading = true
					go func() {
						stockBefore := Stocks[state.Player]
						ts := state.Trade(queue[0])
						for _, card := range ts.Their.Cards {
							if stockBefore[card] == 0 {
								state.Say("clockwork", fmt.Sprintf("I've just aquired %s.", card))
								stockBefore[card]++
							}
						}

						queue = queue[1:]
						if len(queue) > 0 {
							state.Say("clockwork", fmt.Sprintf("Finished trading. %d more in line. Next one is %s", len(queue), queue[0]))
						} else {
							state.Say("clockwork", "Finished trading.")
							currentlyTrading = false
						}
						chReadyToTrade <- true
					}()
				}

			case m := <-state.Listen():
				if strings.HasPrefix(m.Text, "!price") {
					msg := ""

					cardName := matchCardName(strings.Replace(m.Text, "!price ", "", 1))
					price, ok := Prices[cardName]
					if !ok {
						msg = "There is no card named '" + cardName + "'"
					} else {
						stocked := Stocks[state.Player][cardName]
						if stocked > 3 {
							price.Buy = int(float64(price.Buy) * math.Pow(0.85, float64(stocked-3)))
						}
						if stocked < 4 {
							price.Sell = int(float64(price.Sell) * math.Pow(1.05, float64(4-stocked)))
						}
						if stocked == 0 {
							msg = fmt.Sprintf("%s is out of stock. I'm buying for %dg.", cardName, price.Buy)
						} else {
							msg = fmt.Sprintf("I'm buying %s for %dg and selling for %dg (%d stocked).", cardName, price.Buy, price.Sell, stocked)
						}
					}

					if m.Channel == "WHISPER" {
						state.Whisper(m.From, msg)
					} else {
						state.Say(m.Channel, fmt.Sprintf("%s", msg))
					}
				}

				if m.Text == "!stock" {
					commons := 0
					uncommons := 0
					rares := 0
					uniques := make(map[string]bool)

					for _, card := range Libraries[state.Player].Cards {
						name := CardTypes[CardId(card.TypeId)]
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

					state.Say(m.Channel, fmt.Sprintf("I have %d commons, %d uncommons and %d rares. That's %d%% of all card types, as well as %d gold.",
						commons, uncommons, rares, 100*len(uniques)/len(CardTypes), state.Gold))
				}

				if m.Text == "!help" && m.Channel != state.CurrentTradeRoom {
					msg := "Did you mean '!trade' or '!price [scroll name]'? You can also check the !stock."
					if msg != "" {
						if m.Channel == "WHISPER" {
							state.Whisper(m.From, msg)
						} else {
							state.Say(m.Channel, fmt.Sprintf("%s: %s", m.From, msg))
						}
					}
				}

				// if m.Text == "!changelog" {
				// 	state.Say(m.Channel, "Added !changelog. Woo!")
				// 	state.Say(m.Channel, "!price now works in whisper")
				// }

				if m.Text == "!trade" || m.Text == "!queue" {
					msg := ""
					for i, player := range queue {
						if player == m.From {
							msg = fmt.Sprintf("You are already queued for trading. Your position in the queue is %d.", i)
							break
						}
					}
					if msg == "" {
						queue = append(queue, m.From)
						if len(queue) == 1 && !currentlyTrading {
							chReadyToTrade <- true
						} else {
							msg = fmt.Sprintf("You are now queued for trading. Your position in the queue is %d.", len(queue)-1)
						}
					}
					if msg != "" {
						if m.Channel == "WHISPER" {
							state.Whisper(m.From, msg)
						} else {
							state.Say(m.Channel, fmt.Sprintf("%s: %s", m.From, msg))
						}
					}
				}
			}
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			chKillThread <- true
			time.Sleep(time.Second * 5)
			startBot("I live again!")
			return
		}

		chKillThread <- true
		time.Sleep(time.Second * 5)
		startBot("I live again!")
	}()

	for {
		timeout := time.After(time.Minute * 2)
		ticker := time.Tick(30 * time.Second)
	InnerLoop:
		for {
			select {
			case <-chAlive:
				break InnerLoop
			case <-state.chQuit:
				logMessage("!!!QUIT!!!")
				return
			case <-timeout:
				logMessage("!!!TIMEOUT!!!")
				return
			case <-ticker:
				logMessage("..tick..\n")
			}
		}
	}
}

func LoadPrices() {
	resp, err := http.Get("http://www.scrollsguide.com/trade")
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	var b bytes.Buffer
	_, err = io.Copy(&b, resp.Body)

	s := string(b.Bytes())
	re := regexp.MustCompile("<td class='row1 ex'>([A-Z][A-Za-z ]+)+</td><td class='row1'>([0-9]+)g</td><td class='row1'>([0-9]+)g</td>")
	found := re.FindAllStringSubmatch(s, -1)

	for _, matches := range found {
		name := matches[1]
		buy, _ := strconv.Atoi(matches[2])
		sell, _ := strconv.Atoi(matches[3])
		Prices[name] = struct{ Buy, Sell int }{buy, sell}
	}
}

func logTrade(ts TradeStatus) {
	file, err := os.OpenFile("/home/stargazer/go/src/ScrollsTradeBot/tradeLog.txt", os.O_WRONLY+os.O_APPEND, 0)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	io.WriteString(file, fmt.Sprintf("%s: Traded with %s.\nTheir offer: [%dg] %s\nMy offer: [%dg] %s\n\n",
		time.Now().String(), ts.Partner,
		ts.Their.Gold, strings.Join(ts.Their.Cards, ","),
		ts.My.Gold, strings.Join(ts.My.Cards, ",")))
}
