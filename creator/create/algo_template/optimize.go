package main

import (
	"time"

	"github.com/tantralabs/yantra"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/optimize"
)

var searchParameters map[string]models.SearchParameter

func optimizeAlgo() {
	searchParameters = map[string]models.SearchParameter{}
	min, max := optimize.GetMinMaxSearchDomain(searchParameters)
	optimize.DiffEvoOptimize(evaluate, min, max)
}

func evaluate(X []float64) float64 {
	// Use a temporary variable _searchParameters to avoid shared memory for concurrent testing
	_searchParameters := optimize.ConstrainSearchParameters(searchParameters, X)

	algo := CreateAlgo("TEMPLATE-opt", "bitmex", symbol)

	tradingEngine := yantra.NewTradingEngine(&algo, -1)

	start := time.Date(2019, 01, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2020, 04, 01, 0, 0, 0, 0, time.UTC)
	tradingEngine.RunTest(start, end, Rebalance, SetupData)
	// algo = tradingEngine.RunTest()
	return -tradingEngine.Algo.Result.Score
}
