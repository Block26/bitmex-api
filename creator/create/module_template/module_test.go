package module

import (
	"testing"
	"time"

	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra"
	"github.com/tantralabs/yantra/models"
)

const di = 15

func TestModule(t *testing.T) {
	symbol := "XBTUSD"

	config := models.AlgoConfig{
		Name:            "di-test",
		Exchange:        "bitmex",
		Symbol:          symbol,
		DataLength:      60,
		StartingBalance: 100,
	}

	algo := yantra.CreateNewAlgo(config)

	SetParameters(&algo, Params{}, symbol)

	tradingEngine := yantra.NewTradingEngine(&algo, -1)
	start := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 03, 01, 0, 0, 0, 0, time.UTC)
	tradingEngine.RunTest(start, end, rebalance, SetupData)

	expectedScore := -100.0
	if algo.Result.Score != expectedScore {
		t.Error("Sharpe has changed from", expectedScore, "to", algo.Result.Score)
	}

}

// This rebalance is created to simulate an algo using this module
func rebalance(algo *models.Algo) {
	for _, ms := range algo.Account.MarketStates {
		tenMin, tenMinIndex := ms.OHLCV.GetOHLCVData(10)
		fiveMin, fiveMinIndex := ms.OHLCV.GetOHLCVData(5)
		if tenMin.Close[tenMinIndex] > fiveMin.Close[fiveMinIndex] {
			order := iex.Order{
				Market:   ms.Symbol,
				Currency: ms.Symbol,
				Amount:   1,
				Rate:     ms.LastPrice,
				Type:     "market",
				Side:     "buy",
			}
			algo.Client.PlaceOrder(order)
		} else {
			order := iex.Order{
				Market:   ms.Symbol,
				Currency: ms.Symbol,
				Amount:   1,
				Rate:     ms.LastPrice,
				Type:     "market",
				Side:     "sell",
			}
			algo.Client.PlaceOrder(order)
		}
		// if ms.Position < 0 {

		// }
	}
}
