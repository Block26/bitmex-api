package algo

import (
	"log"
	"math"
	"sort"

	"github.com/tantralabs/tradeapi/iex"
)

func deltaFloat(a, b, delta float64) bool {
	return math.Abs(a-b) <= delta
}

func (a *Algo) PlaceOrdersOnBook(ex iex.IExchange, openOrders []iex.WSOrder) {

	// For now. Should be parameterized
	qtyTolerance := 1.0
	priceTolerance := 1.0

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
				Amount:   float64(int(totalQty)),
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
				Amount:   float64(int(totalQty)),
				Rate:     toFixed(orderPrice, 2),
				Type:     "Limit",
				Side:     "Sell",
			}
			asks = append(asks, order)
			totalQty = 0.0
		}
	}

	// Get open buys, buys, open sells, sells, with matches filtered out
	var openBuys []iex.WSOrder
	var openSells []iex.WSOrder

	for _, order := range openOrders {
		if order.Side == "Buy" {
			openBuys = append(openBuys, order)
		} else if order.Side == "Sell" {
			openSells = append(openSells, order)
		}
	}

	// Make a local sifting function
	siftMatches := func(open []iex.WSOrder, new []iex.Order) ([]iex.WSOrder, []iex.Order) {
		openfound := make([]bool, len(open))
		newfound := make([]bool, len(new))

		/*
			Not 100% efficient, but it's simple and predictable (more hardware friendly,
			which will likely make it more efficient). O(kn) time, but k and n should both
			be pretty small anyway.
		*/
		for i, op := range open {
			for j, nw := range new {
				if (deltaFloat(op.Price, nw.Rate, priceTolerance)) && (deltaFloat(op.OrderQty, nw.Amount, qtyTolerance)) {
					openfound[i] = true
					newfound[j] = true
				}
			}
		}

		var retOpen []iex.WSOrder
		var retNew []iex.Order
		// Filter out matches
		for i, op := range open {
			if !openfound[i] {
				retOpen = append(retOpen, op)
			}
		}
		for i, nw := range new {
			if !newfound[i] {
				retNew = append(retNew, nw)
			}
		}
		return retOpen, retNew
	}

	// Call local sifting function to get rid of matches
	openBuys, bids = siftMatches(openBuys, bids)
	openSells, asks = siftMatches(openSells, asks)

	// Sort buy and sell orders by priority
	sort.Slice(bids, func(a, b int) bool {
		return bids[a].Rate > bids[b].Rate
	})
	sort.Slice(asks, func(a, b int) bool {
		return asks[a].Rate < asks[b].Rate
	})

	sort.Slice(openBuys, func(a, b int) bool {
		return openBuys[a].Price > openBuys[b].Price
	})
	sort.Slice(openSells, func(a, b int) bool {
		return openSells[a].Price < openSells[b].Price
	})

	cancel := func(order iex.WSOrder) {
		log.Println("Trying to cancel", order.OrderID)
		err := ex.CancelOrder(iex.CancelOrderF{
			Market: order.Symbol,
			Uuid:   order.OrderID,
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	place := func(order iex.Order) {
		uuid, err := ex.PlaceOrder(order)
		if err != nil {
			log.Fatal(err)
		} else {
			log.Println("Placed BUY", uuid)
		}
	}

	byix := 0
	slix := 0
	bcont := len(bids) != 0
	scont := len(asks) != 0
	for bcont || scont {
		if bcont && scont {
			diffb := math.Abs(bids[byix].Rate - a.Market.Price)
			diffs := math.Abs(asks[slix].Rate - a.Market.Price)
			if diffb < diffs {
				// cancel buy
				if len(openBuys) > byix {
					cancel(openBuys[byix])
					place(bids[byix])
					byix++
				}
			} else {
				// cancel sell
				if len(openSells) > slix {
					cancel(openSells[slix])
					place(asks[slix])
					slix++
				}
			}
		} else if !bcont {
			// finish the rest of the sells
			for i := slix; i < len(asks); i++ {
				cancel(openSells[i])
				place(asks[i])
			}
			break
		} else if !scont {
			// finish the rest of the buys
			for i := byix; i < len(bids); i++ {
				cancel(openBuys[i])
				place(bids[i])
			}
			break
		}
		bcont = (byix < len(bids))
		scont = (slix < len(asks))
	}
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
