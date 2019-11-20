package algo

import (
	"log"
	"math"
	"sort"
	"strings"

	"github.com/tantralabs/tradeapi/iex"
)

func deltaFloat(a, b, delta float64) bool {
	return math.Abs(a-b) <= delta
}

func (a *Algo) PlaceOrdersOnBook(ex iex.IExchange, openOrders []iex.WSOrder) {

	// For now. Should be parameterized
	qtyTolerance := 0.0001
	priceTolerance := 1.0

	var bids []iex.Order
	var asks []iex.Order
	totalQty := 0.0
	for i, qty := range a.Market.BuyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Market.MinimumOrderSize {
			orderPrice := a.Market.BuyOrders.Price[i]
			order := iex.Order{
				Market:   a.Market.BaseAsset.Symbol,
				Currency: a.Market.QuoteAsset.Symbol,
				Amount:   totalQty, //float64(int(totalQty)),
				Rate:     toFixed(orderPrice, 8),
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
				Market:   a.Market.BaseAsset.Symbol,
				Currency: a.Market.QuoteAsset.Symbol,
				Amount:   totalQty, //float64(int(totalQty)),
				Rate:     toFixed(orderPrice, 8),
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

	log.Println("openOrders", openOrders)
	for _, order := range openOrders {
		if strings.ToLower(order.Side) == "buy" {
			openBuys = append(openBuys, order)
		} else if strings.ToLower(order.Side) == "sell" {
			openSells = append(openSells, order)
		}
	}
	log.Println("openSells", openSells)

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

	buyIndex := 0
	sellIndex := 0
	buyCont := len(bids) != 0
	sellCont := len(asks) != 0
	for buyCont || sellCont {
		if buyCont && sellCont {
			buyDiff := math.Abs(bids[buyIndex].Rate - a.Market.Price)
			sellDiff := math.Abs(asks[sellIndex].Rate - a.Market.Price)
			if buyDiff < sellDiff {
				// cancel buy
				if len(openBuys) > buyIndex {
					cancel(openBuys[buyIndex])
					place(bids[buyIndex])
					buyIndex++
				}
			} else {
				// cancel sell
				if len(openSells) > sellIndex {
					cancel(openSells[sellIndex])
					place(asks[sellIndex])
					sellIndex++
				}
			}
		} else if !buyCont {
			// finish the rest of the sells
			for i := sellIndex; i < len(openSells); i++ {
				cancel(openSells[i])
			}

			for i := sellIndex; i < len(asks); i++ {
				place(asks[i])
			}
		} else if !sellCont {
			// finish the rest of the buys
			for i := buyIndex; i < len(openBuys); i++ {
				cancel(openBuys[i])
			}
			for i := sellIndex; i < len(bids); i++ {
				place(bids[i])
			}
			break
		}
		buyCont = (buyIndex < len(bids))
		sellCont = (sellIndex < len(asks))
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
