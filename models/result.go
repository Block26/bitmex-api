package models

// The Result struct contains information about a backtest result.
type Result struct {
	Balance           float64 // Ending balance for the backtest
	DailyReturn       float64 // Average daily return (in percent)
	MaxLeverage       float64 // Max leverage used by the algo during the backtest
	MaxPositionProfit float64 // Max profit of a single position in the backtest
	MaxPositionDD     float64 // Max drawdown of a single position in the backtest
	MaxDD             float64 // Total max drawdown during the backtest
	Score             float64 // Sharpe ratio of the entire backtest
	RollingScore      float64 // Rolling Sharpe ratio of backtest, determined by algo.RollingInterval
	Sortino           float64 // Sortino ratio of the entire backtest
	Params            string  // Algo params for this backtest
}
