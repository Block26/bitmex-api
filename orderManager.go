package algo

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/tantralabs/tradeapi/iex"
)

func deltaFloat(a, b, delta float64) bool {
	return math.Abs(a-b) <= delta
}

func (a *Algo) PlaceOrdersOnBook(ex iex.IExchange, openOrders []iex.WSOrder) {

	// For now. Should be parameterized
	qtyTolerance := 1.0
	priceTolerance := 1.0

	var newBids []iex.Order
	var newAsks []iex.Order

	createBid := func(i int, qty float64) {
		orderPrice := a.Market.BuyOrders.Price[i]
		quantity := ToFixed(qty, a.Market.QuantityPrecision)
		quantity = RoundToNearest(qty, float64(a.Market.QuantityTickSize))
		order := iex.Order{
			Market:   a.Market.BaseAsset.Symbol,
			Currency: a.Market.QuoteAsset.Symbol,
			Amount:   quantity,
			Rate:     ToFixed(orderPrice, a.Market.PricePrecision),
			Type:     "Limit",
			Side:     "Buy",
		}
		newBids = append(newBids, order)
	}

	createAsk := func(i int, qty float64) {
		orderPrice := a.Market.SellOrders.Price[i]
		quantity := ToFixed(qty, a.Market.QuantityPrecision)
		quantity = RoundToNearest(qty, float64(a.Market.QuantityTickSize))
		order := iex.Order{
			Market:   a.Market.BaseAsset.Symbol,
			Currency: a.Market.QuoteAsset.Symbol,
			Amount:   quantity,
			Rate:     ToFixed(orderPrice, a.Market.PricePrecision),
			Type:     "Limit",
			Side:     "Sell",
		}
		newAsks = append(newAsks, order)
	}

	totalQty := 0.0
	for i, qty := range a.Market.BuyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Market.MinimumOrderSize {
			createBid(i, totalQty)
			totalQty = 0.0
		}
	}

	if totalQty > 0.0 {
		index := len(a.Market.BuyOrders.Quantity) - 1
		createBid(index, totalQty)
	}

	totalQty = 0.0
	for i, qty := range a.Market.SellOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > a.Market.MinimumOrderSize {
			createAsk(i, totalQty)
			totalQty = 0.0
		}
	}

	if totalQty > 0.0 {
		index := len(a.Market.SellOrders.Quantity) - 1
		createAsk(index, totalQty)
	}

	//Parse option orders
	totalQty = 0.0
	for _, option := range a.Market.Options {
		for i, qty := range option.BuyOrders.Quantity {
			fmt.Printf("Parsing order for option %v: price %v qty %v\n", option.OptionTheo.String(), i, qty)
			totalQty += qty
			if totalQty >= option.MinimumOrderSize {
				orderPrice := option.BuyOrders.Price[i]
				// Assume a price of 0 indicates market order
				var orderType string
				if orderPrice == 0 {
					orderType = "Market"
				} else {
					orderType = "Limit"
				}
				order := iex.Order{
					Market:   option.Symbol,
					Currency: a.Market.QuoteAsset.Symbol,
					Amount:   ToFixed(totalQty, 1), //float64(int(totalQty)),
					Rate:     ToFixed(orderPrice, a.Market.PricePrecision),
					Type:     orderType,
					Side:     "Buy",
				}
				newBids = append(newBids, order)
				totalQty = 0.0
			}
		}
	}
	totalQty = 0.0
	for _, option := range a.Market.Options {
		for i, qty := range option.SellOrders.Quantity {
			fmt.Printf("Parsing order for option %v: price %v qty %v\n", option.OptionTheo.String(), i, qty)
			totalQty += qty
			if totalQty >= option.MinimumOrderSize {
				orderPrice := option.SellOrders.Price[i]
				// Assume a price of 0 indicates market order
				var orderType string
				if orderPrice == 0 {
					orderType = "Market"
				} else {
					orderType = "Limit"
				}
				order := iex.Order{
					Market:   option.Symbol,
					Currency: a.Market.QuoteAsset.Symbol,
					Amount:   ToFixed(totalQty, 1), //float64(int(totalQty)),
					Rate:     ToFixed(orderPrice, a.Market.PricePrecision),
					Type:     orderType,
					Side:     "Sell",
				}
				newAsks = append(newAsks, order)
				totalQty = 0.0
			}
		}
	}

	log.Println("New orders")
	log.Println(newAsks)
	log.Println(newBids)
	// Get open buys, buys, open sells, sells, with matches filtered out
	var openBids []iex.WSOrder
	var openAsks []iex.WSOrder

	for _, order := range openOrders {
		if strings.ToLower(order.Side) == "buy" {
			openBids = append(openBids, order)
		} else if strings.ToLower(order.Side) == "sell" {
			openAsks = append(openAsks, order)
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
	openBids, newBids = siftMatches(openBids, newBids)
	openAsks, newAsks = siftMatches(openAsks, newAsks)

	// Sort buy and sell orders by priority
	sort.Slice(newBids, func(a, b int) bool {
		return newBids[a].Rate > newBids[b].Rate
	})
	sort.Slice(newAsks, func(a, b int) bool {
		return newAsks[a].Rate < newAsks[b].Rate
	})

	sort.Slice(openBids, func(a, b int) bool {
		return openBids[a].Price > openBids[b].Price
	})
	sort.Slice(openAsks, func(a, b int) bool {
		return openAsks[a].Price < openAsks[b].Price
	})

	cancel := func(order iex.WSOrder) {
		// log.Println("Trying to cancel", order.OrderID)
		err := ex.CancelOrder(iex.CancelOrderF{
			Market: order.Symbol,
			Uuid:   order.OrderID,
		})
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	place := func(order iex.Order) {
		_, err := ex.PlaceOrder(order)
		if err != nil {
			log.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	bidIndex := 0
	askIndex := 0
	buyCont := len(newBids) != 0
	sellCont := len(newAsks) != 0

	// fmt.Printf("Num buys %v, num sells %v\n", len(newBids), len(newAsks))

	for buyCont || sellCont {
		if buyCont && sellCont {
			buyDiff := math.Abs(newBids[bidIndex].Rate - a.Market.Price)
			sellDiff := math.Abs(newAsks[askIndex].Rate - a.Market.Price)
			if buyDiff < sellDiff {
				// cancel buy
				if len(openBids) > bidIndex {
					cancel(openBids[bidIndex])
				}
				if len(newBids) > bidIndex {
					place(newBids[bidIndex])
				}
				bidIndex++
			} else {
				// cancel sell
				if len(openAsks) > askIndex {
					cancel(openAsks[askIndex])
				}
				if len(newAsks) > askIndex {
					place(newAsks[askIndex])
				}
				askIndex++
			}
		} else {
			fmt.Printf("Else\n")
			// finish the rest of the orders

			if a.Market.BulkCancelSupported {
				cancelStr := ""
				for i := askIndex; i < len(openAsks); i++ {
					cancelStr += openAsks[i].OrderID + ","
				}

				for i := bidIndex; i < len(openBids); i++ {
					cancelStr += openBids[i].OrderID + ","
				}

				cancelStr = strings.TrimSuffix(cancelStr, ",")
				if len(cancelStr) > 0 {
					log.Println("Trying to bulk cancel")
					log.Println(cancelStr)
					err := ex.CancelOrder(iex.CancelOrderF{
						Market: a.Market.Symbol,
						Uuid:   cancelStr,
					})

					if err != nil {
						log.Fatal(err)
					}
				}
			} else {
				for i := askIndex; i < len(openAsks); i++ {
					cancel(openAsks[i])
				}

				for i := bidIndex; i < len(openBids); i++ {
					cancel(openBids[i])
				}
			}
			// fmt.Printf("Newasks: %v, newbids: %v\n", newAsks, newBids)
			// fmt.Printf("Ask index: %v\n", askIndex)
			for i := askIndex; i < len(newAsks); i++ {
				place(newAsks[i])
			}
			// fmt.Printf("Bid index: %v\n", bidIndex)
			for i := bidIndex; i < len(newBids); i++ {
				place(newBids[i])
			}
			break
		}
		buyCont = (bidIndex < len(newBids))
		sellCont = (askIndex < len(newAsks))
	}
}

func UpdateLocalOrders(oldOrders []iex.WSOrder, newOrders []iex.WSOrder) []iex.WSOrder {
	var updatedOrders []iex.WSOrder
	for _, oldOrder := range oldOrders {
		found := false
		for _, newOrder := range newOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
				if newOrder.OrdStatus == orderStatus.Cancelled || newOrder.OrdStatus == orderStatus.Filled || newOrder.OrdStatus == orderStatus.Rejected {
					log.Println(newOrder.OrdStatus, oldOrder.OrderID)
				} else {
					updatedOrders = append(updatedOrders, newOrder)
				}
			}
		}
		if !found {
			if oldOrder.OrdStatus == orderStatus.Cancelled || oldOrder.OrdStatus == orderStatus.Filled || oldOrder.OrdStatus == orderStatus.Rejected {
				log.Println(oldOrder.OrdStatus, oldOrder.OrderID)
			} else {
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
			if newOrder.OrdStatus == orderStatus.Cancelled || newOrder.OrdStatus == orderStatus.Filled || newOrder.OrdStatus == orderStatus.Rejected {
				log.Println(newOrder.OrdStatus, newOrder.OrderID)
			} else {
				updatedOrders = append(updatedOrders, newOrder)
			}
		}
	}

	log.Println(len(updatedOrders), "orders")
	return updatedOrders
}
