package main

import (
	"GoMarketMaker/models"
	"log"
	"math"
)

func rebalance(price float64, averageCost float64, quantity float64) (models.OrderArray, models.OrderArray) {
	liquid := 0.05 //Defined as btc but will be % in the future
	var buyOrders, sellOrders models.OrderArray

	// Create Buy Orders
	buying := liquid * price
	if quantity < 0 {
		buying = buying + math.Abs(quantity)
		startBuyPrice := price
		if averageCost < price {
			startBuyPrice = averageCost
		}
		buyOrders = createSpread(1, 2, startBuyPrice, 0.005, 2, settings.MaxOrders)
	} else {
		buyOrders = createSpread(1, 2, price, 0.01, 2, settings.MaxOrders)
	}
	log.Println("Placing", buying, "on bid")
	buyOrders.Quantity = mulArr(buyOrders.Quantity, buying)

	// Create Sell orders
	selling := liquid * price
	if quantity > 0 {
		selling = selling + quantity
		startSellPrice := price
		if averageCost > price {
			startSellPrice = averageCost
		}
		sellOrders = createSpread(-1, 0.1, startSellPrice, 0.005, 2, settings.MaxOrders)
	} else {
		sellOrders = createSpread(-1, 2, price, 0.01, 2, settings.MaxOrders)
	}

	log.Println("Placing", selling, "on ask")
	sellOrders.Quantity = mulArr(sellOrders.Quantity, selling)

	return buyOrders, sellOrders
}
