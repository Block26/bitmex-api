package database

import (
	"log"
	"sort"

	"github.com/tantralabs/logger"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/models"
)

var barData []*models.Bar

func GetBars() []*models.Bar {
	return barData
}

func UpdateLocalBars(localBars *[]*models.Bar, newBars []*models.Bar) {
	timestamps := make([]int64, len(barData))
	for i := range barData {
		timestamps[i] = barData[i].Timestamp
	}

	if newBars != nil {
		for y := range newBars {
			if !containsInt(timestamps, newBars[y].Timestamp) {
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
	sort.Slice(b, func(i, j int) bool { return (*localBars)[i].Timestamp > (*localBars)[j].Timestamp })
	localBars = &b
}

func UpdateBars(ex iex.IExchange, symbol string, bin string, count int) []*models.Bar {
	newData, err := ex.GetData(symbol, bin, count)
	if err != nil {
		log.Fatal("err getting data", err)
	}

	timestamps := make([]int64, len(barData))
	for i := range barData {
		timestamps[i] = barData[i].Timestamp
	}

	for y := range newData {
		if !containsInt(timestamps, newData[y].Timestamp.Unix()*1000) {
			barData = append(barData, &models.Bar{
				Timestamp: newData[y].Timestamp.Unix() * 1000,
				Open:      newData[y].Open,
				High:      newData[y].High,
				Low:       newData[y].Low,
				Close:     newData[y].Close,
			})
		}
	}

	sort.Slice(barData, func(i, j int) bool { return barData[i].Timestamp < barData[j].Timestamp })
	return barData
}

func GetLatestMinuteData(ex iex.IExchange, symbol string, exchange string, dataLength int) []*models.Bar {
	logger.Info("Fetching", dataLength, "1m Data for symbol:", symbol, "exchange:", exchange, "from our db")
	barData = GetCandles(symbol, exchange, "1m", dataLength)
	logger.Info("Fetching", 240, "1m Data for symbol:", symbol, "exchange:", exchange, "from the exchange")
	exchangeBars := UpdateBars(ex, symbol, "1m", 240) // 4hour buffer
	UpdateLocalBars(&barData, exchangeBars)
	return barData
}

func containsInt(s []int64, e int64) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
