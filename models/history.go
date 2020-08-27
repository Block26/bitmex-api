package models

import "time"

// Generic History struct for summarizing the state of the algo at a given point in time.
type History struct {
	Timestamp    time.Time
	Symbol       string
	Balance      float64
	UBalance     float64
	QuoteBalance float64
	Quantity     float64
	AverageCost  float64
	Leverage     float64
	Profit       float64
	Weight       int
	MaxLoss      float64
	MaxProfit    float64
	Price        float64
}

type TrimmedHistory struct {
	Timestamp time.Time
	Leverage  float64
	Score     float64
}

// Represents the state of account balances at a given point in time.
type BalanceHistory struct {
	Timestamp string  `csv:"timestamp"`
	Balance   float64 `csv:"balance"`
	UBalance  float64 `csv:"u_balance"`
}
