package main

import (
	"math"

	"github.com/block26/exchanges/models"
)

type Asset struct {
	BaseBalance float64
	Quantity    float64
	AverageCost float64
	Profit      float64
	Fee         float64
	TickSize    float64
	Delta       float64
	Buying      float64
	Selling     float64
	MaxOrders   int32
	Leverage    float64
	MaxLeverage float64
}

type Algo struct {
	//Required
	Asset      Asset
	Futures    bool
	Debug      bool
	BuyOrders  models.OrderArray
	SellOrders models.OrderArray
	//Custom
	EntrySpread     float64
	EntryConfidence float64
	ExitSpread      float64
	ExitConfidence  float64
	Liquidity       float64
}

func (a *Algo) rebalance(price float64) {
	// Create Buy Orders
	a.Asset.Buying = a.Liquidity * a.Asset.BaseBalance // * price
	if a.Asset.Quantity < 0 {
		a.Asset.Buying = a.Asset.Buying + (math.Abs(a.Asset.Quantity) / price)
		startBuyPrice := price
		if a.Asset.Leverage < a.Asset.MaxLeverage {
			if a.Asset.AverageCost < price {
				startBuyPrice = a.Asset.AverageCost
			}
		}
		a.BuyOrders = createSpread(1, a.ExitConfidence, startBuyPrice, a.ExitSpread, a.Asset.TickSize, a.Asset.MaxOrders)
	} else {
		a.BuyOrders = createSpread(1, a.EntryConfidence, price, a.EntrySpread, a.Asset.TickSize, a.Asset.MaxOrders)
	}
	// buyOrders.Quantity = mulArr(buyOrders.Quantity, buying)

	// Create Sell orders
	a.Asset.Selling = a.Liquidity * a.Asset.BaseBalance // * price
	if a.Asset.Quantity > 0 {
		a.Asset.Selling = a.Asset.Selling + (a.Asset.Quantity / price)
		startSellPrice := price
		if a.Asset.Leverage < a.Asset.MaxLeverage {
			if a.Asset.AverageCost > price {
				startSellPrice = a.Asset.AverageCost
			}
		}
		a.SellOrders = createSpread(-1, a.ExitConfidence, startSellPrice, a.ExitSpread, a.Asset.TickSize, a.Asset.MaxOrders)
	} else {
		a.SellOrders = createSpread(-1, a.EntryConfidence, price, a.EntrySpread, a.Asset.TickSize, a.Asset.MaxOrders)
	}

	// sellOrders.Quantity = mulArr(sellOrders.Quantity, selling)

}
