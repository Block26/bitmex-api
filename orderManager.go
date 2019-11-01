package algo

import (
	"log"

	"github.com/block26/exchanges/bitmex/swagger"
	"gitlab.com/raedah/tradeapi/iex"
)

func (a *Algo) PlaceOrdersOnBook(openOrders iex.OpenOrders) ([]iex.Order, []string) {
	currentOrders := append(openOrders.Bids, openOrders.Asks...)
	var orders []iex.Order
	totalQty := 0.0
	for _, qty := range a.BuyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Asset.MinimumOrderSize {
			// orderPrice := a.BuyOrders.Price[i]
			order := iex.Order{} //a.Asset.Symbol, totalQty, orderPrice, "Limit"}
			orders = append(orders, order)
			totalQty = 0.0
		}
	}

	totalQty = 0.0
	for _, qty := range a.SellOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Asset.MinimumOrderSize {
			// orderPrice := a.SellOrders.Price[i]
			order := iex.Order{} //a.Asset.Symbol, totalQty, orderPrice, "Limit"}
			orders = append(orders, order)
			totalQty = 0.0
		}
	}

	var toCreate []iex.Order
	var orderToPlace []float64
	for _, newOrder := range orders {
		if newOrder.Type != "Market" {
			orderFound := false
			for _, oldOrder := range currentOrders {
				// If we are trying to place the same order then just leave the current one
				if !orderFound && oldOrder.Price == newOrder.Rate && oldOrder.Quantity == newOrder.Amount {
					orderFound = true
					orderToPlace = append(orderToPlace, newOrder.Rate)
					break
				}
			}
			if !orderFound {
				// now := time.Now().Unix() - 1524872844
				toCreate = append(toCreate, newOrder)
				// log.Println("Found order", newOrder.ClOrdID)
				orderToPlace = append(orderToPlace, newOrder.Rate)
			}
		}
	}

	var toCancel []string
	for _, oldOrder := range currentOrders {
		found := false
		for _, newOrder := range orderToPlace {
			if newOrder == oldOrder.Price {
				found = true
				break
			}
		}
		if !found {
			toCancel = append(toCancel, oldOrder.UUID)
		}
	}

	// return toCreate, toAmend, toCancel
	// Cancel first?
	// Should consider cancel/create in 10 order blocks so cancel 10 then create the 10 to replace
	// b.CancelOrders(toCancel, 0)
	// b.CreateOrders(toCreate, 0)
	return toCreate, toCancel
}

func UpdateLocalOrders(oldOrders []*swagger.Order, newOrders []*swagger.Order) []*swagger.Order {
	var updatedOrders []*swagger.Order
	for _, oldOrder := range oldOrders {
		found := false
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
			log.Println("Adding Order", newOrder.OrderID, newOrder.OrdStatus)
		}
	}

	log.Println(len(updatedOrders), "orders")
	return updatedOrders
}
