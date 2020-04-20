package models

import (
	"log"
	"sync"
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

// Representation of the state of a given market.
type MarketState struct {
	Symbol           string
	Info             MarketInfo
	Position         float64
	AverageCost      float64
	UnrealizedProfit float64
	RealizedProfit   float64
	Profit           float64
	Leverage         float64
	Weight           int
	Balance          float64  // We want this balance to track the account balance automatically
	Orders           sync.Map // [orderId]Order
	LastPrice        float64
	MidMarketPrice   float64
	BestBid          float64
	BestAsk          float64
	Bar              Bar   // The last bar of data for this market
	OHLCV            *Data // Open, High, Low, Close, Volume Data
	Status           MarketStatus

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

func (ms *MarketState) GetCurrentWeight() int {
	if ms.Position > 0 {
		return 1
	} else if ms.Position < 0 {
		return -1
	}
	return 0
}

func NewMarketState(marketInfo MarketInfo, balance float64) MarketState {
	var syncMap sync.Map
	return MarketState{
		Symbol:  marketInfo.Symbol,
		Info:    marketInfo,
		Balance: balance,
		Orders:  syncMap,
	}
}

func NewMarketStateFromExchange(symbol string, exchangeInfo ExchangeInfo, balance float64) MarketState {
	marketInfo, err := LoadMarketInfo(exchangeInfo.Exchange, symbol)
	if err != nil {
		log.Fatal("Error loading market info")
	}
	var syncMap sync.Map
	return MarketState{
		Symbol:  symbol,
		Info:    marketInfo,
		Balance: balance,
		Orders:  syncMap,
	}
}

func NewMarketStateFromInfo(marketInfo MarketInfo, balance float64) MarketState {
	var syncMap sync.Map
	return MarketState{
		Symbol:  marketInfo.Symbol,
		Info:    marketInfo,
		Balance: balance,
		Orders:  syncMap,
	}
}
