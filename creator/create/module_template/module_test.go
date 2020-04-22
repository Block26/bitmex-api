package module

import (
	"testing"
	"time"

	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra"
	"github.com/tantralabs/yantra/models"
)

func TestModule(t *testing.T) {
	symbol := "XBTUSD"

	algo := yantra.CreateNewAlgo(models.AlgoConfig{
		Name:            "di-test",
		Exchange:        "bitmex",
		Symbol:          symbol,
		DataLength:      1,
		StartingBalance: 100,
	})

	SetParameters(&algo, Params{}, symbol)

	tradingEngine := yantra.NewTradingEngine(&algo, -1)
	start := time.Date(2019, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	tradingEngine.RunTest(start, end, rebalance, SetupData)

	expectedScore := 1.785
	if algo.Result.Score != expectedScore {
		t.Error("Sharpe has changed from", expectedScore, "to", algo.Result.Score)
	}

}

// This rebalance is created to simulate an algo using this module
func rebalance(algo *models.Algo) {
	for _, ms := range algo.Account.MarketStates {
		if ms.Position < 0 {
			order := iex.Order{
				Market:   ms.Symbol,
				Currency: ms.Symbol,
				Amount:   1,
				Rate:     ms.LastPrice,
				Type:     "market",
				Side:     "buy",
			}
			algo.Client.PlaceOrder(order)
		}
	}
}
