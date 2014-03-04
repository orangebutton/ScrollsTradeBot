package main

import (
	"math"
	"strconv"
	"strings"
)

func Levenshtein(a, b string) int {
	var cost int
	d := make([][]int, len(a)+1)
	for i := 0; i < len(d); i++ {
		d[i] = make([]int, len(b)+1)
	}
	var min1, min2, min3 int

	for i := 0; i < len(d); i++ {
		d[i][0] = i
	}

	for i := 0; i < len(d[0]); i++ {
		d[0][i] = i
	}
	for i := 1; i < len(d); i++ {
		for j := 1; j < len(d[0]); j++ {
			if a[i-1] == b[j-1] {
				cost = 0
			} else {
				cost = 1
			}

			min1 = d[i-1][j] + 1
			min2 = d[i][j-1] + 1
			min3 = d[i-1][j-1] + cost
			d[i][j] = int(math.Min(math.Min(float64(min1), float64(min2)), float64(min3)))
		}
	}

	return d[len(a)][len(b)]
}

func matchCardName(input string) []Card {
	minDist := 2
	bestFits := make([]Card, 0)

	if input == "" {
		return bestFits
	}

	for _, card := range CardTypes {
		dist := Levenshtein(strings.ToLower(input), strings.ToLower(string(card)))
		if dist <= minDist {
			minDist = dist
			bestFits = append(bestFits, card)
		}
	}

	if minDist > 0 {
		alternativeFits := make([]Card, 0)

		for _, card := range CardTypes {
			for _, substr := range strings.Split(string(card), " ") {
				if substr == input {
					alternativeFits = append(alternativeFits, card)
				}
			}
		}

		if len(alternativeFits) == 0 {
			for _, card := range CardTypes {
				if strings.Contains(strings.ToLower(string(card)), strings.ToLower(input)) {
					alternativeFits = append(alternativeFits, card)
				}
			}
		}

		if len(alternativeFits) == 1 {
			return alternativeFits
		} else {
			bestFits = append(bestFits, alternativeFits...)
		}
	}

	// we don't want to allow too nonspecific terms
	if len(bestFits) > 4 {
		bestFits = make([]Card, 0)
	}

	return bestFits
}

func parseCardList(str string) (cards map[Card]int, ambiguousWords, failedWords []string) {
	cards = make(map[Card]int)
	failedWords = make([]string, 0)

	for _, word := range strings.Split(str, ",") {
		word = reInvalidChars.ReplaceAllString(strings.ToLower(word), "")

		num := 1
		match := reNumbers.FindStringSubmatch(word)
		if len(match) == 2 {
			num, _ = strconv.Atoi(match[1])
			word = reNumbers.ReplaceAllString(word, "")
		}
		matchedCards := matchCardName(strings.Trim(word, " "))
		switch len(matchedCards) {
		case 0:
			failedWords = append(failedWords, word)
		case 1:
			cards[matchedCards[0]] = num
		default:
			ambiguousWords = append(ambiguousWords, word)
		}
	}
	return
}

func listify(cards []Card, thing string) string {
	stringies := make([]string, len(cards))
	for i, card := range cards {
		stringies[i] = string(card)
	}

	switch len(cards) {
	case 0:
		return ""
	case 1:
		return stringies[0]
	default:
		return strings.Join(stringies[0:len(stringies)-1], ", ") + thing + stringies[len(stringies)-1]
	}
}

func andify(cards []Card) string {
	return listify(cards, " and ")
}

func orify(cards []Card) string {
	return listify(cards, " or ")
}
