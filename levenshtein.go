package main

import "math"

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
