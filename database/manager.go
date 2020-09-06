package database

import (
	"log"
	"sort"

	"github.com/tantralabs/logger"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/models"
)

var BarData []*models.Bar

func GetBars() []*models.Bar {
	return BarData
}

func UpdateLocalBars(localBars *[]*models.Bar, newBars []*models.Bar) {
	timestamps := make([]int64, len(BarData))
	for i := range BarData {
		timestamps[i] = BarData[i].Timestamp
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

	timestamps := make([]int64, len(BarData))
	for i := range BarData {
		timestamps[i] = BarData[i].Timestamp
	}

	for y := range newData {
		if !containsInt(timestamps, newData[y].Timestamp.Unix()*1000) {
			BarData = append(BarData, &models.Bar{
				Timestamp: newData[y].Timestamp.Unix() * 1000,
				Open:      newData[y].Open,
				High:      newData[y].High,
				Low:       newData[y].Low,
				Close:     newData[y].Close,
			})
		}
	}

	sort.Slice(BarData, func(i, j int) bool { return BarData[i].Timestamp < BarData[j].Timestamp })
	return BarData
}

func GetLatestMinuteData(ex iex.IExchange, symbol string, exchange string, dataLength int) []*models.Bar {
	logger.Info("Fetching", dataLength, "1m Data for symbol:", symbol, "exchange:", exchange, "from our db")
	BarData = GetCandles(symbol, exchange, "1m", dataLength)
	logger.Info("Fetching", 240, "1m Data for symbol:", symbol, "exchange:", exchange, "from the exchange")
	exchangeBars := UpdateBars(ex, symbol, "1m", 240) // 4hour buffer
	// logger.Infof("Loaded %v instances of exchangeBars for %v with start %v and end %v.\n", len(exchangeBars), symbol, utils.TimestampToTime(int(exchangeBars[0].Timestamp)), utils.TimestampToTime(int(exchangeBars[len(exchangeBars)-1].Timestamp)))
	UpdateLocalBars(&BarData, exchangeBars)
	// logger.Infof("Loaded %v instances of bar data for %v with start %v and end %v.\n", len(BarData), symbol, utils.TimestampToTime(int(BarData[0].Timestamp)), utils.TimestampToTime(int(BarData[len(BarData)-1].Timestamp)))
	return BarData
}

func GetLatestMinuteDataFromExchange(ex iex.IExchange, symbol string, exchange string, dataLength int) []*models.Bar {
	BarData := UpdateBars(ex, symbol, "1m", 240) // 4hour buffer
	return BarData
}

func containsInt(s []int64, e int64) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
