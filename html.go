package main

import (
	"fmt"
	"math"
	"net/http"
	"sort"
)

func startWebServer() {
	http.HandleFunc("/scrolls", func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte(showPriceTable(currentState)))
		deny(err)
	})

	err := http.ListenAndServe(":8080", nil)
	deny(err)
}

type StockedItem struct {
	card    Card
	stocked int
	buy     int
	sell    int
}
type SortBySellPrice []StockedItem

func (a SortBySellPrice) Len() int           { return len(a) }
func (a SortBySellPrice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a SortBySellPrice) Less(i, j int) bool { return a[i].sell > a[j].sell }

func showPriceTable(s *State) string {
	if s == nil {
		return "Bot is offline."
	}

	toSort := make(SortBySellPrice, 0)
	for card, stocked := range Stocks[Bot] {
		toSort = append(toSort, StockedItem{card, stocked, s.DeterminePrice(card, 1, true), s.DeterminePrice(card, 1, false)})
	}
	sort.Sort(toSort)

	out := `
	<html>
		<head>
			<title>ClockworkAgent SGPrices</title>
		</head>
		<body>
			<b><h1>ClockworkAgent - SGPrices and FAQ</h1></b>
			
			<b>What is this?</b><br>
			This site displays the SGPrices for which the ClockworkAgent, a trading bot for the game Scrolls, will buy and sell cards for.<br><br>
			
			<b>How do I engage in trading with the bot?</b><br>
			Just join the ingame channel "clockwork" and say "!trade". You will then be queued up for interaction with the bot in a trade.<br><br>
			
			<b>Why are these SGPrices so different from Scrollsguide SGPrices?</b><br>
			Scrollsguide SGPrices are determined from WTB and WTS messages in the Trading-channel. Thus they reflect what people expect to pay/get
			for a card, not necessarily what the card is actually traded for. Since most people adjust their expectations to what the current Scrollsguide
			price is, this can lead to a self-fulfilling prophecy. Also, it is pretty easy to manipulate the SGPrices for cards that are traded less often,
			enabling a way to scam the bot if it would use these SGPrices.<br><br>

			<b>How then are these SGPrices calculated?</b><br>
			The price starts at 1200 for rares, 600 for uncommons and 150 for commons. Each time a card is sold to the bot, it will assume that
			the card is less valuable, reducing the price by 100 / 50 / 12.5 depending on rarity. Each time a card is bought from the bot, the price will
			go up again.<br><br>

			<b>Why does the buy price fluctuate, when the sell price remains constant?</b><br>` +
		fmt.Sprintf("If the bot has less than 2000 gold, the buy SGPrices will be reduced by up to 50%%. The bot currently has %d gold, reducing SGPrices by %.0f%%. ",
			GoldForTrade(), (1.0-(math.Min(float64(Gold), 10000.0)/20000.0+0.5))*100) +
		`This way the bot can aquire more cards, balancing out the fact that it had to overpay for most cards in order to determine the price, as well as
		 new additions and general price deflation.<br><br>

			<b>Who created the bot, and where can I find the source code?</b><br>
			You can find the source at <a href="https://github.com/redefiance/ScrollsTradeBot">Github</a> and me occasionally in Scrolls under the name
			<i>redefiance</i>.<br><br>

			<b>Why does this site look like crap?</b><br>
			I am not a web designer.<br><br>

			<table border="1" align="center">
				<tr>
					<th>Scroll name (# stocked)</th>
					<th>Buying for</th>
					<th>Selling for</th>
				<tr>
	`
	for _, item := range toSort {
		out += fmt.Sprintf("<tr><td align='right'>%s (%d)</td><td align='center'>%d</td><td align='center'>%d</td></tr>\n",
			item.card, item.stocked, item.buy, item.sell)
	}

	out += `
			</table>
		</body>
	</html>
	`

	return out
}
