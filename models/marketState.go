package models

import (
	"log"

	"github.com/tantralabs/tradeapi/iex"
)

type MarketStatus int

const (
	Open MarketStatus = iota
	Expired
	Closed
)

var marketStatuses = [...]string{
	"Open",
	"Expired",
	"Closed",
}

// Representation of the state of a given market. It contains static market information as well as
// dynamic state that changes with market data.
type MarketState struct {
	Symbol           string     // symbol of a given market
	Info             MarketInfo // static information about the underlying contract
	Position         float64    // our current position (can be positive or negative)
	AverageCost      float64    // average cost of our current position (zero if no position)
	UnrealizedProfit float64
	RealizedProfit   float64
	Profit           float64 // unrealized profit + realized profit
	Leverage         float64
	Weight           int                  // sign of desired position (SHOULD BE MOVED)
	Balance          float64              // balance assigned to the given market (total account balance if using cross margin)
	UBalance         float64              // unrealized balance assigned to the given market (total account balance if using cross margin)
	Orders           map[string]iex.Order // [orderId]Order
	LastPrice        float64              // the last trade price observed in the given market
	MidMarketPrice   float64              // average of bid and ask (not yet tracked)
	BestBid          float64              // (not yet tracked)
	BestAsk          float64              // (not yet tracked)
	Bar              Bar                  // the last bar of data for this market
	OHLCV            *Data                // Open, High, Low, Close, Volume Data
	Status           MarketStatus         // is this market, open, closed, or expired?

	// Only for options
	OptionTheo *OptionTheo

	CanBuyBasedOnMax   bool    // If true then yantra will calculate leverage based on Market.MaxLeverage, if false then yantra will calculate leverage based on Algo.LeverageTarget
	ShouldHaveQuantity float64 // Keeps track of the order sizing when live.
	ShouldHaveLeverage float64 // Keeps track of the order sizing when live.
	MaxLeverage        float64
}

// Returns the current sign of the position for this market.
func (ms *MarketState) GetCurrentWeight() int {
	if ms.Position > 0 {
		return 1
	} else if ms.Position < 0 {
		return -1
	}
	return 0
}

// COnstructor for a new market state given a symbol and general exchange information.
func NewMarketStateFromExchange(symbol string, exchangeInfo ExchangeInfo, balance float64) MarketState {
	marketInfo, err := LoadMarketInfo(exchangeInfo.Exchange, symbol)
	if err != nil {
		log.Fatal("Error loading market info")
	}
	return MarketState{
		Symbol:  symbol,
		Info:    marketInfo,
		Balance: balance,
		Orders:  make(map[string]iex.Order),
	}
}

// Constructor for a new market state given static market information and a base account balance.
func NewMarketStateFromInfo(marketInfo MarketInfo, balance float64) MarketState {
	return MarketState{
		Symbol:  marketInfo.Symbol,
		Info:    marketInfo,
		Balance: balance,
		Orders:  make(map[string]iex.Order),
	}
}
