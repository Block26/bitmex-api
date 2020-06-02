package models

import (
	"github.com/tantralabs/tradeapi/iex"
)

// The Trade struct contains information regarding a single historical trade.
type Trade struct {
	ID        string  // Unique trade identifier
	Symbol    string  // Market symbol for this trade
	Amount    float64 // Volume traded between buyer and seller
	Price     float64 // The price of the maker order for this trade
	Side      string  // The side of the taker order for this trade ("buy" or "sell")
	MakerID   string  // Order id for the maker order
	TakerID   string  // Order id for the taker order
	Timestamp int     // Exchange timestamp at time of trade
}

// Construct a new trade from a single order and exchange timestamp.
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
