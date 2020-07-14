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

	// These variables should be module specific; we'll leave them here for now
	// AutoOrderPlacement
	// AutoOrderPlacement is not neccesary and can be false, it is the easiest way to create an algorithm with yantra
	// using AutoOrderPlacement will allow yantra to automatically leverage and deleverage your account based on Algo.Market.Weight
	//
	// Examples
	// EX) If RebalanceInterval().Hour and EntryOrderSize = 0.1 then when you are entering a position you will order 10% of your LeverageTarget per hour.
	// EX) If RebalanceInterval().Hour and ExitOrderSize = 0.1 then when you are exiting a position you will order 10% of your LeverageTarget per hour.
	// EX) If RebalanceInterval().Hour and DeleverageOrderSize = 0.01 then when you are over leveraged you will order 1% of your LeverageTarget per hour until you are no longer over leveraged.
	// EX) If Market.MaxLeverage is 1 and Algo.LeverageTarget is 1 then your algorithm will be fully leveraged when it enters it's position.
	AutoOrderPlacement  bool    // AutoOrderPlacement whether yantra should manage your orders / leverage for you.
	CanBuyBasedOnMax    bool    // If true then yantra will calculate leverage based on Market.MaxLeverage, if false then yantra will calculate leverage based on Algo.LeverageTarget
	FillPrice           float64 // The price at which the algo thinks it filled in the backtest
	FillShift           int     // The simulation fill shift for this Algo. 0 = filling at beginning of interval, 1 = filling at end of interval
	LeverageTarget      float64 // The target leverage for the Algo, 1 would be 100%, 0.5 would be 50% of the MaxLeverage defined by Market.
	EntryOrderSize      float64 // The speed at which the algo enters positions during the RebalanceInterval
	ExitOrderSize       float64 // The speed at which the algo exits positions during the RebalanceInterval
	DeleverageOrderSize float64 // The speed at which the algo exits positions during the RebalanceInterval if it is over leveraged, current leverage is determined by Algo.LeverageTarget or Market.MaxLeverage.
	ShouldHaveQuantity  float64 // Keeps track of the order sizing when live.
	MaxLeverage         float64
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
