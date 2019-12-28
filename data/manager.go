package data

import (
	"log"
	"sort"

	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/tradeapi/iex"
)

var barData []*models.Bar

func UpdateLocalBars(localBars *[]*models.Bar, newBars []*models.Bar) {
	timestamps := make([]string, len(*localBars))
	for i := range *localBars {
		timestamps[i] = (*localBars)[i].Timestamp.String()
	}

	if newBars != nil {
		for y := range newBars {
			if !containsString(timestamps, newBars[y].Timestamp.String()) {
				newBars = append(newBars, &models.Bar{
					Timestamp: newBars[y].Timestamp,
					Open:      newBars[y].Open,
					High:      newBars[y].High,
					Low:       newBars[y].Low,
					Close:     newBars[y].Close,
				})
			}
		}
	}

	var b []*models.Bar
	sort.Slice(b, func(i, j int) bool { return (*localBars)[i].Timestamp.After((*localBars)[j].Timestamp) })
	localBars = &b
}

func UpdateBars(ex iex.IExchange, symbol string, bin string, count int) []*models.Bar {
	newData, err := ex.GetData(symbol, bin, count)
	if err != nil {
		log.Fatal("err getting data", err)
	}

	timestamps := make([]string, len(barData))
	for i := range barData {
		timestamps[i] = barData[i].Timestamp.String()
	}

	for y := range newData {
		if !containsString(timestamps, newData[y].Timestamp.String()) {
			barData = append(barData, &models.Bar{
				Timestamp: newData[y].Timestamp,
				Open:      newData[y].Open,
				High:      newData[y].High,
				Low:       newData[y].Low,
				Close:     newData[y].Close,
			})
		}
	}

	sort.Slice(barData, func(i, j int) bool { return barData[i].Timestamp.Before(barData[j].Timestamp) })
	return barData
}

func containsString(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
