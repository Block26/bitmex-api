package main

import (
	"log"
	"math"

	"github.com/block26/exchanges/models"
)

type Asset struct {
	BaseBalance float64
	Quantity    float64
	AverageCost float64
	Profit      float64
	Fee         float64
	MinTickSize float64
	Delta       float64
	Buying      float64
	Selling     float64
	MaxOrders   int32
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
	MaxLeverage     float64
}

func (a *Algo) rebalance(price float64) {
	liquid := 0.05 //Defined as btc but will be % in the future
	// Create Buy Orders
	a.Asset.Buying = liquid * price
	if a.Asset.Quantity < 0 {
		a.Asset.Buying = a.Asset.Buying + math.Abs(a.Asset.Quantity)
		startBuyPrice := price
		if a.Asset.AverageCost < price {
			startBuyPrice = a.Asset.AverageCost
		}
		a.BuyOrders = createSpread(1, 2, startBuyPrice, 0.005, 2, a.Asset.MaxOrders)
	} else {
		a.BuyOrders = createSpread(1, 2, price, 0.01, 2, a.Asset.MaxOrders)
	}
	log.Println("Placing", a.Asset.Buying, "on bid")
	a.BuyOrders.Quantity = mulArr(a.BuyOrders.Quantity, a.Asset.Buying)

	// Create Sell orders
	a.Asset.Selling = liquid * price
	if a.Asset.Quantity > 0 {
		a.Asset.Selling = a.Asset.Selling + a.Asset.Quantity
		startSellPrice := price
		if a.Asset.AverageCost > price {
			startSellPrice = a.Asset.AverageCost
		}
		a.SellOrders = createSpread(-1, 0.1, startSellPrice, 0.005, 2, a.Asset.MaxOrders)
	} else {
		a.SellOrders = createSpread(-1, 2, price, 0.01, 2, a.Asset.MaxOrders)
	}

	log.Println("Placing", a.Asset.Selling, "on ask")
	a.SellOrders.Quantity = mulArr(a.SellOrders.Quantity, a.Asset.Selling)

	// return a.BuyOrders, a.SellOrders
}
