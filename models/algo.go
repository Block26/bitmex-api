package models

import (
	"time"

	"github.com/tantralabs/tradeapi/iex"
)

// Algo is where you will define your initial state and it will keep track of your state throughout a test and during live execution.
type Algo struct {
	Name              string                 // A UUID that tells the live system what algorithm is executing trades
	AccountId         string                 // A UUID for the secret describing the api keys and exchange
	Account           Account                // A representation of the account an algo uses on an exchange; offers access to active markets and market states
	ExchangeInfo      ExchangeInfo           // Contains various information about the exchange this algo is connected to
	FillType          string                 // The simulation fill type for this Algo. Refer to exchanges.FillType() for options
	RebalanceInterval string                 // The interval at which rebalance should be called. Refer to exchanges.RebalanceInterval() for options
	Debug             bool                   // Turn logs on or off
	Index             int                    // Current index of the Algo in it's data
	Timestamp         time.Time              // Current timestamp of the Algo in it's data
	DataLength        int                    // Datalength tells the Algo when it is safe to start rebalancing, your Datalength should be longer than any TA length
	History           []History              // Used to Store historical states
	Params            map[string]interface{} // Save the initial Params of the Algo, for logging purposes. This is used to check the params after running a genetic search.
	Result            map[string]interface{} // The result of your backtest
	Stats             Stats                  // The stats of your backtest
	LogStats          bool                   // Turn logs on or off for stats of your backtest, and exports them to a stats.csv in your local directory
	Signals           map[string][]float64   // Log the signals of your test
	LogBacktestToCSV  bool                   // Exports the backtest history to a balance.csv in your local directory
	State             map[string]interface{} // State of the algo, useful for logging live ta indicators.
	TheoEngine        interface{}            // Daniel's secret sauce
	LogLevel          int
	BacktestLogLevel  int
	Client            iex.IExchange
}