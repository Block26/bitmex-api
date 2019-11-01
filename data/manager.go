package data

import (
	"github.com/block26/TheAlgoV2/models"
)

func UpdateLocalBars(localBars []models.Bar, newBars []models.Bar) []models.Bar {
	timestamps := make([]string, len(localBars))
	for i := range localBars {
		timestamps[i] = localBars[i].Timestamp
	}

	for y := range newBars {
		if !containsString(timestamps, newBars[y].Timestamp) {
			localBars = append(localBars, models.Bar{
				Timestamp: newBars[y].Timestamp,
				Open:      newBars[y].Open,
				High:      newBars[y].High,
				Low:       newBars[y].Low,
				Close:     newBars[y].Close,
			})
		}
	}

	return localBars
}

func containsString(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
