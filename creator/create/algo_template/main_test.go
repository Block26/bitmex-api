package main

import (
	"testing"
	"time"
)

func TestAlgo(t *testing.T) {
	// SETUP ALGO
	symbol := "XBTUSD"

	tradingEngine := BuildTradingEngine("TEMPLATE-test", "bitmex", "XBTUSD")
	start := time.Date(2019, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
	tradingEngine.RunTest(start, end, rebalance, SetupData)

	expectedScore := 1.785
	if algo.Result.Score != expectedScore {
		t.Error("Sharpe has changed from", expectedScore, "to", algo.Result.Score)
	}
}
