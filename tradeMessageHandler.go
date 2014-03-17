// tradeMessageHandler
package main

import (
	"fmt"
	"strings"
)

const helpText string = "Add the scrolls you want to sell on your side. " +
	"To buy scrolls from me, say 'wtb [list of scrolls]'. " +
	"You can also !add or !remove single cards. " +
	"If you want to start over you can always !reset. " +
	"You may want to know the !price of a card. " +
	"A !donation is always welcome."

func (s *State) TradeMessageHandler(donation bool, m Message, tradePartner Player, ts TradeStatus) bool {

	command, args := ParseCommandAndArgs(m)
	switch command {
	case "!help":
		s.handleTradeHelp(m.Channel)
	case "!add", "!wtb":
		s.handleAdd(ts, tradePartner, args)
	case "!remove":
		s.handleRemove(command, ts, m.Channel)
	case "!reset":
		s.handleReset(ts)
	case "!price":
		s.handleTradePrice(ts, m.Channel)
	case "!donation":
		donation = !donation
		s.handleDonation(donation, m.Channel)
	}
	return donation
}

func (s *State) handleTradeHelp(tradeRoom Channel) {
	s.Say(tradeRoom, helpText)
}

func (s *State) handleAdd(ts TradeStatus, tradePartner Player, args string) {

	requestedCards, ambiguousWords, failedWords := parseCardList(args)
	cardIds := make([]CardUid, 0)

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
			reply += fmt.Sprintf("I don't know what '%s' is. ", failedWords)
		}
		if reply != "" {
			s.Say(TradeRoom, reply)
		}
		if len(cardIds) > 0 {
			s.SendRequest(Request{"msg": "TradeAddCards", "cardIds": cardIds})
		}
	}
}

func (s *State) handleRemove(args string, ts TradeStatus, tradeRoom Channel) {
	if len(args) == 0 {
		s.Say(TradeRoom, "You have to name the card that I will remove.")
	}

	params := strings.TrimPrefix(args, "!remove ")
	matchedCards := matchCardName(params)
	switch len(matchedCards) {
	case 0:
		s.Say(tradeRoom, fmt.Sprintf("There is no scroll named '%s'.", params))
	case 1:
		card := matchedCards[0]
		alreadyOffered := ts.My.Cards[card]
		if alreadyOffered == 0 {
			s.Say(tradeRoom, fmt.Sprintf("%s is not part of this trade!", card))
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
		s.Say(tradeRoom, fmt.Sprintf("'%s' is %s.", params, matchCardName(params)))
	}
}

func (s *State) handleReset(ts TradeStatus) {
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
}

func (s *State) handleTradePrice(ts TradeStatus, tradeRoom Channel) {
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
	s.Say(tradeRoom, msg)
}

func (s *State) handleDonation(donation bool, tradeRoom Channel) {
	if donation {
		s.Say(TradeRoom, "I will consider everything you put into this trade as a donation. Much appreciated!"+
			" If you change your mind, just repeat the command.")
	} else {
		s.Say(TradeRoom, "Okay :(")
	}
}
