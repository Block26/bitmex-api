package main

import (
	"os"
	"time"

	"github.com/tantralabs/yantra"
	"github.com/tantralabs/yantra/models"
)

func CreateAlgo(name string, exchange string, symbol string) models.Algo {
	config := models.AlgoConfig{
		Name:            name,
		Exchange:        exchange,
		Symbol:          symbol,
		DataLength:      1,
		StartingBalance: 100,
	}
	return yantra.CreateNewAlgo(config)
}

func BuildTradingEngine(name string, exchange string, symbol string) (tradingEngine yantra.TradingEngine) {
	// Instantiate algo
	algo := CreateAlgo(name, exchange, symbol)
	// Build trading engine and database
	tradingEngine = yantra.NewTradingEngine(&algo, -1)
	return
}

// The main function should setup and then run your algo, you will pass an arg using the values
// live, optimize, or test which will tell your algo what to do.
func main() {
	// READ CLI ARGS
	var opt string
	if len(os.Args) > 1 {
		opt = os.Args[1]
	} else {
		opt = "test"
	}

	// SETUP TRADING ENGINE

	// RUN STRATEGY LIVE
	if opt == "live" {
		config := models.LoadConfig("config.json")
		tradingEngine := BuildTradingEngine(config.Name, config.Exchange, config.Symbol)
		tradingEngine.Connect(config.Secret, true, Rebalance, SetupData)
		return
	} else if opt == "optimize" {
		optimizeAlgo()
	}

	tradingEngine := BuildTradingEngine("TEMPLATE-test", "bitmex", "XBTUSD")

	start := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 01, 02, 0, 0, 0, 0, time.UTC)
	tradingEngine.RunTest(start, end, Rebalance, SetupData)
}

// SetupData is called at the beggining of the strategy and is used to setup and preprocess
// data and signals to be used by the algo later during rebalancing
func SetupData(algo *models.Algo) {
}

// Rebalance will be called at every row in your data set, rebalance is where the core logic of your
// Algo should be placed
func Rebalance(algo *models.Algo) {
}
