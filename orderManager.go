package algo

import (
	"log"

	"github.com/tantralabs/tradeapi/iex"
)

func (a *Algo) PlaceOrdersOnBook(ex iex.IExchange, openOrders []iex.WSOrder) ([]iex.Order, []iex.WSOrder) {
	var orders []iex.Order
	totalQty := 0.0
	//TODO
	for i, qty := range a.BuyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Asset.MinimumOrderSize {
			orderPrice := a.BuyOrders.Price[i]
			order := iex.Order{
				Market: a.Asset.Symbol,
				Amount: float64(int(totalQty)),
				Rate:   orderPrice,
				Type:   "Buy",
			}
			orders = append(orders, order)
			totalQty = 0.0
		}
	}

	totalQty = 0.0
	for i, qty := range a.SellOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Asset.MinimumOrderSize {
			orderPrice := a.SellOrders.Price[i]
			order := iex.Order{
				Market: a.Asset.Symbol,
				Amount: float64(int(totalQty)),
				Rate:   orderPrice,
				Type:   "Sell",
			}
			orders = append(orders, order)
			totalQty = 0.0
		}
	}

	var toCreate []iex.Order
	var orderToPlace []float64

	for _, newOrder := range orders {
		if newOrder.Type != "Market" {
			orderFound := false
			for _, oldOrder := range openOrders {
				if !orderFound && oldOrder.Price == newOrder.Rate && oldOrder.OrderQty == newOrder.Amount {
					// If we are trying to place the same order then just leave the current one
					orderFound = true
					orderToPlace = append(orderToPlace, newOrder.Rate)
					log.Println("Found an order for that already")
					break
				} else if !orderFound && oldOrder.Price == newOrder.Rate {
					// If we are trying to place the same order with a different quantity
					// then we should cancel it and place the new order
					orderFound = true
					orderToPlace = append(orderToPlace, newOrder.Rate)
					log.Println(iex.CancelOrderF{
						Market: oldOrder.Symbol,
						Uuid:   oldOrder.OrderID,
					})
					err := ex.CancelOrder(iex.CancelOrderF{
						Market: oldOrder.Symbol,
						Uuid:   oldOrder.OrderID,
					})
					if err != nil {
						log.Fatal(err)
					}
					if newOrder.Type == "Buy" {
						uuid, err := ex.BuyLimit(newOrder)
						if err != nil {
							log.Fatal(err)
						} else {
							log.Println("Placed BUY", uuid)
						}
					} else {
						uuid, err := ex.SellLimit(newOrder)
						if err != nil {
							log.Fatal(err)
						} else {
							log.Println("Placed SELL", uuid)
						}
					}
					break
				}
			}
			if !orderFound {
				if newOrder.Type == "Buy" {
					uuid, err := ex.BuyLimit(newOrder)
					if err != nil {
						log.Fatal(err)
					} else {
						log.Println("Placed BUY", uuid)
					}
				} else {
					uuid, err := ex.SellLimit(newOrder)
					if err != nil {
						log.Fatal(err)
					} else {
						log.Println("Placed SELL", uuid)
					}
				}
				orderToPlace = append(orderToPlace, newOrder.Rate)
			}
		}
	}

	var toCancel []iex.WSOrder
	for _, oldOrder := range openOrders {
		found := false
		for _, newOrder := range orderToPlace {
			if newOrder == oldOrder.Price {
				found = true
				break
			}
		}
		if !found {
			// toCancel = append(toCancel, oldOrder)
			log.Println("Trying to cancel", oldOrder.OrderID)
			log.Println(iex.CancelOrderF{
				Market: oldOrder.Symbol,
				Uuid:   oldOrder.OrderID,
			})
			err := ex.CancelOrder(iex.CancelOrderF{
				Market: oldOrder.Symbol,
				Uuid:   oldOrder.OrderID,
			})
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Canceled", oldOrder.OrderID)
		}
	}

	// log.Println(len(toCreate), "toCreate")
	// log.Println(len(toCancel), "toCancel")

	return toCreate, toCancel
}

func UpdateLocalOrders(oldOrders []iex.WSOrder, newOrders []iex.WSOrder) []iex.WSOrder {
	var updatedOrders []iex.WSOrder
	// log.Println(len(newOrders), "new orders")
	for _, oldOrder := range oldOrders {
		found := false
		// log.Println("oldOrder.OrdStatus", oldOrder.OrdStatus)
		for _, newOrder := range newOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
				if newOrder.OrdStatus == "Canceled" || newOrder.OrdStatus == "Filled" || newOrder.OrdStatus == "Rejected" {
					log.Println(newOrder.OrdStatus, oldOrder.OrderID)
				} else {
					updatedOrders = append(updatedOrders, newOrder)
					// log.Println("Updated Order", newOrder.OrderID, newOrder.OrdStatus)
				}
			}
		}
		if !found {
			if oldOrder.OrdStatus == "Canceled" || oldOrder.OrdStatus == "Filled" || oldOrder.OrdStatus == "Rejected" {
				log.Println(oldOrder.OrdStatus, oldOrder.OrderID)
			} else {
				// log.Println("Old Order", oldOrder.OrderID)
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
			// log.Println("Adding Order", newOrder.OrderID, newOrder.OrdStatus)
		}
	}

	// log.Println(len(updatedOrders), "orders")
	return updatedOrders
}
