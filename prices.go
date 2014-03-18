// prices
package main

import (
	"encoding/json"
	"io/ioutil"
	"math"
	"net/http"
)

type Price struct{ Buy, Sell int }

var SGPrices = make(map[Card]Price)

// these prices are set by the store
func StoreValue(card Card) int {
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

func MyMinValue(card Card) int {
	switch CardRarities[card] {
	case 0:
		return 50
	case 1:
		return 150
	case 2:
		return 300
	}
	return -1
}

func MyMaxValue(card Card) int {
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

func sellValue(card Card, stocked int) float64 {
	minPrice := float64(MyMinValue(card))
	basePrice := float64(MyMaxValue(card))

	discount := 1.0 - (float64(stocked-1))/Conf.MaxNumToBuy
	price := math.Max(minPrice, (basePrice * discount))
	return math.Floor(price)
}

func buyValue(card Card, stocked int) float64 {
	goldFactor := math.Min(float64(GoldForTrade()), Conf.GoldThreshold)/(Conf.GoldThreshold*2) + 0.5

	minPrice := float64(StoreValue(card))
	basePrice := float64(MyMaxValue(card))

	discount := 1.0 - (float64(stocked-1))/Conf.MaxNumToBuy
	price := math.Max(minPrice, (basePrice * discount * goldFactor))
	return math.Floor(price)
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
	price := 0.0
	stocked := Stocks[Bot][card]

	for i := 0; i < num; i++ {
		if buy {
			price += buyValue(card, stocked)
			stocked++
		} else {
			price += sellValue(card, stocked)
			stocked--
		}
	}
	return int(price)
}

func autobotsPricing(card Card, num int, buy bool) int {
	price := 0
	if buy {
		price = num * StoreValue(card)
	} else {
		price = pricingBasedOnInventory(card, num, buy)
	}
	return price
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
		p := Price{Buy: StoreValue(name), Sell: MyMaxValue(name)}
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
