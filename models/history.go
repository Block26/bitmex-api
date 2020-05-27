package models

import "time"

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

type BalanceHistory struct {
	Timestamp string  `csv:"timestamp"`
	Balance   float64 `csv:"balance"`
	UBalance  float64 `csv:"u_balance"`
}
