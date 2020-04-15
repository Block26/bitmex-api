package models

import (
	"github.com/tantralabs/tradeapi/iex"
)

type Trade struct {
	ID        string
	Symbol    string
	Amount    float64
	Price     float64
	Side      string
	MakerID   string
	TakerID   string
	Timestamp int
}

func NewTradeFromOrder(order iex.Order, timestamp int) Trade {
	var side string
	if order.Type == "limit" {
		if order.Side == "buy" {
			side = "sell"
		} else {
			side = "buy"
		}
	} else {
		side = order.Side
	}
	return Trade{
		ID:        "",
		Symbol:    order.Market,
		Amount:    order.Amount,
		Price:     order.Rate,
		Side:      side,
		Timestamp: timestamp,
	}
}
