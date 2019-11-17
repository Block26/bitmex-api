package algo

import (
	"log"

	"github.com/tantralabs/tradeapi/iex"
)

func (a *Algo) PlaceOrdersOnBook(ex iex.IExchange, openOrders []iex.WSOrder) {
	var bids []iex.Order
	var asks []iex.Order
	totalQty := 0.0
	for i, qty := range a.Market.BuyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Market.MinimumOrderSize {
			orderPrice := a.Market.BuyOrders.Price[i]
			order := iex.Order{
				Market:   a.BaseAsset.Symbol,
				Currency: a.QuoteAsset.Symbol,
				Amount:   totalQty,
				Rate:     toFixed(orderPrice, 2),
				Type:     "Limit",
				Side:     "Buy",
			}
			bids = append(bids, order)
			totalQty = 0.0
		}
	}

	totalQty = 0.0
	for i, qty := range a.Market.SellOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Market.MinimumOrderSize {
			orderPrice := a.Market.SellOrders.Price[i]
			order := iex.Order{
				Market:   a.BaseAsset.Symbol,
				Currency: a.QuoteAsset.Symbol,
				Amount:   totalQty,
				Rate:     toFixed(orderPrice, 2),
				Type:     "Limit",
				Side:     "Sell",
			}
			asks = append(asks, order)
			totalQty = 0.0
		}
	}

	// var toCreate []iex.Order
	// var orderToPlace []float64

	_, bidsToCreate, _ := sortOrders(bids, openOrders)
	// bidsToPlace, bidsToCreate, bidsToCancel := sortOrders(bids, openOrders)
	log.Println(bidsToCreate)

	_, asksToCreate, _ := sortOrders(asks, openOrders)
	// asksToPlace, asksToCreate, asksToCancel := sortOrders(asks, openOrders)
	log.Println(asksToCreate)

	// for _, newOrder := range orders {
	// 	if newOrder.Type != "Market" {
	// 		orderFound := false
	// 		for _, oldOrder := range openOrders {
	// 			if !orderFound && oldOrder.Price == newOrder.Rate && oldOrder.OrderQty == newOrder.Amount {
	// 				// If we are trying to place the same order then just leave the current one
	// 				orderFound = true
	// 				orderToPlace = append(orderToPlace, newOrder.Rate)
	// 				break
	// 			} else if !orderFound && oldOrder.Price == newOrder.Rate {
	// 				// If we are trying to place the same order with a different quantity
	// 				// then we should cancel it and place the new order
	// 				log.Println("Cancel then place")
	// 				orderFound = true
	// 				orderToPlace = append(orderToPlace, newOrder.Rate)
	// 				log.Println(iex.CancelOrderF{
	// 					Market: oldOrder.Symbol,
	// 					Uuid:   oldOrder.OrderID,
	// 				})
	// 				err := ex.CancelOrder(iex.CancelOrderF{
	// 					Market: oldOrder.Symbol,
	// 					Uuid:   oldOrder.OrderID,
	// 				})
	// 				if err != nil {
	// 					log.Println("Error:", err)
	// 					time.Sleep(500)
	// 				}
	// 				// log.Println("Canceled", oldOrder.OrderID)
	// 				log.Println("Placing order:", newOrder)
	// 				uuid, err := ex.PlaceOrder(newOrder)
	// 				if err != nil {
	// 					log.Println("Error:", err)
	// 					time.Sleep(1 * time.Second)
	// 				} else {
	// 					log.Println("Placed order:", newOrder, uuid)
	// 					time.Sleep(150 * time.Millisecond)
	// 				}
	// 				break
	// 			}
	// 		}
	// 		if !orderFound {
	// 			log.Println("Placing order:", newOrder)
	// 			uuid, err := ex.PlaceOrder(newOrder)
	// 			if err != nil {
	// 				log.Println("Error:", err)
	// 				time.Sleep(1 * time.Second)
	// 			} else {
	// 				log.Println("Placed order:", uuid)
	// 				time.Sleep(150 * time.Millisecond)
	// 			}
	// 			orderToPlace = append(orderToPlace, newOrder.Rate)
	// 		}
	// 	}
	// }

	// var toCancel []iex.WSOrder
	// for _, oldOrder := range openOrders {
	// 	found := false
	// 	for _, newOrder := range orderToPlace {
	// 		if newOrder == oldOrder.Price {
	// 			found = true
	// 			break
	// 		}
	// 	}
	// 	if !found {
	// 		// toCancel = append(toCancel, oldOrder)
	// 		log.Println("Trying to cancel", oldOrder.OrderID)
	// 		err := ex.CancelOrder(iex.CancelOrderF{
	// 			Market: oldOrder.Symbol,
	// 			Uuid:   oldOrder.OrderID,
	// 		})
	// 		if err != nil {
	// 			log.Println("Error:", err)
	// 		}
	// 		// log.Println("Canceled", oldOrder.OrderID)
	// 		time.Sleep(150 * time.Millisecond)
	// 	}
	// }

	// log.Println(len(toCreate), "toCreate")
	// log.Println(len(toCancel), "toCancel")

	return
}

func sortOrders(orders []iex.Order, openOrders []iex.WSOrder) ([]float64, []iex.Order, []iex.WSOrder) {
	var toCreate []iex.Order
	var toCancel []iex.WSOrder
	var orderToPlace []float64
	for _, newOrder := range orders {
		if newOrder.Type != "Market" {
			orderFound := false
			for _, oldOrder := range openOrders {
				if !orderFound && oldOrder.Price == newOrder.Rate && oldOrder.OrderQty == newOrder.Amount {
					// If we are trying to place the same order then just leave the current one
					orderFound = true
					orderToPlace = append(orderToPlace, newOrder.Rate)
					break
				} else if !orderFound && oldOrder.Price == newOrder.Rate {
					// If we are trying to place the same order with a different quantity
					// then we should cancel it and place the new order
					orderFound = true
					orderToPlace = append(orderToPlace, newOrder.Rate)
					toCreate = append(toCreate, newOrder)
					toCancel = append(toCancel, oldOrder)
					break
				}
			}
			if !orderFound {
				toCreate = append(toCreate, newOrder)
				orderToPlace = append(orderToPlace, newOrder.Rate)
			}
		}
	}

	return orderToPlace, toCreate, toCancel
}

func UpdateLocalOrders(oldOrders []iex.WSOrder, newOrders []iex.WSOrder) []iex.WSOrder {
	var updatedOrders []iex.WSOrder
	// log.Println(len(oldOrders), "old orders")
	// log.Println(len(newOrders), "new orders")
	for _, oldOrder := range oldOrders {
		found := false
		// log.Println("oldOrder.OrdStatus", oldOrder.OrdStatus)
		for _, newOrder := range newOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
				if newOrder.OrdStatus == orderStatus.Cancelled || newOrder.OrdStatus == orderStatus.Filled || newOrder.OrdStatus == orderStatus.Rejected {
					log.Println(newOrder.OrdStatus, oldOrder.OrderID)
				} else {
					updatedOrders = append(updatedOrders, newOrder)
					// log.Println("Updated Order", newOrder.OrderID, newOrder.OrdStatus)
				}
			}
		}
		if !found {
			if oldOrder.OrdStatus == orderStatus.Cancelled || oldOrder.OrdStatus == orderStatus.Filled || oldOrder.OrdStatus == orderStatus.Rejected {
				log.Println(oldOrder.OrdStatus, oldOrder.OrderID)
			} else {
				// log.Println("Old Order", oldOrder.OrderID, oldOrder.OrdStatus)
				updatedOrders = append(updatedOrders, oldOrder)
			}
		}
	}

	for _, newOrder := range newOrders {
		found := false
		for _, oldOrder := range oldOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
			}
		}
		if !found {
			updatedOrders = append(updatedOrders, newOrder)
			log.Println("Adding Order", newOrder.OrderID, newOrder.OrdStatus)
		}
	}

	// log.Println(len(updatedOrders), "orders")
	return updatedOrders
}
