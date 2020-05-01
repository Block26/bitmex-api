package main

import (
	"testing"
	"time"
)

func TestAlgo(t *testing.T) {
	// SETUP ALGO
	tradingEngine := BuildTradingEngine("TEMPLATE-test", "bitmex", "XBTUSD")
	start := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 01, 02, 0, 0, 0, 0, time.UTC)
	tradingEngine.RunTest(start, end, Rebalance, SetupData)

	expectedScore := -100.0
	if tradingEngine.Algo.Result.Score != expectedScore {
		t.Error("Sharpe has changed from", expectedScore, "to", tradingEngine.Algo.Result.Score)
	}
}
