package yantra

import (
	"log"
	"math"
	"sort"
	"strings"

	"github.com/tantralabs/exchanges"
	"github.com/tantralabs/logger"
	. "github.com/tantralabs/models"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/utils"
)

func deltaFloat(a, b, delta float64) bool {
	return math.Abs(a-b) <= delta
}

func setupOrders(algo *Algo, currentPrice float64) {
	price := currentPrice
	if algo.AutoOrderPlacement {
		orderSize, side := getOrderSize(algo, algo.Market.Price.Close, true)
		if side == 0 {
			return
		}

		// Adjust order size to order over the hour
		if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
			orderSize = (orderSize * (1 + orderSize)) / 60
		}

		var quantity float64
		if algo.Market.Futures {
			quantity = orderSize * (algo.Market.BaseAsset.Quantity * algo.Market.Price.Close)
		} else {
			quantity = orderSize * (algo.Market.BaseAsset.Quantity / algo.Market.Price.Close)
		}

		// Keep track of what we should have so the orders we place will grow and shrink
		algo.ShouldHaveQuantity += quantity * side

		// Get the difference of what we have and what we should have, thats what we should order
		quantityToOrder := algo.ShouldHaveQuantity - algo.Market.QuoteAsset.Quantity

		// Don't over order while adding to the position
		orderSide := math.Copysign(1, quantityToOrder)
		quantitySide := math.Copysign(1, algo.Market.QuoteAsset.Quantity)

		if orderSide == quantitySide && math.Abs(algo.ShouldHaveQuantity) > canBuy(algo) && algo.Market.Leverage < algo.LeverageTarget {
			log.Println("Don't over order while adding to the position")
			algo.ShouldHaveQuantity = canBuy(algo) * quantitySide
			quantityToOrder = algo.ShouldHaveQuantity - algo.Market.QuoteAsset.Quantity
		}

		// When deleveraging to meet canBuy don't go lower than can buy
		// log.Println("quantityToOrder", quantityToOrder, "math.Abs(quantity+(quantityToOrder*side))", math.Abs(quantity+(quantityToOrder*side)), "side", side, (quantityToOrder * side))
		// log.Printf("Weight: %v, quantitySide %v algo.ShouldHaveQuantity %v, canBuy %v\n", algo.Market.Weight, quantitySide, algo.ShouldHaveQuantity, canBuy(algo))
		if (algo.Market.Weight != 0 && algo.Market.Weight == int(quantitySide)) && algo.Market.Leverage > algo.LeverageTarget && math.Abs(algo.Market.QuoteAsset.Quantity+quantityToOrder) < canBuy(algo) {
			algo.ShouldHaveQuantity = canBuy(algo) * quantitySide
			quantityToOrder = (math.Abs(algo.Market.QuoteAsset.Quantity) - canBuy(algo)) * orderSide
			// quantityToOrder = (math.Abs(quantity) - canBuy(algo)) * -quantitySide
			log.Printf("Don't over order when reducing leverage should have qty: %v\n", algo.ShouldHaveQuantity)
		}

		// Don't over order to go neutral
		if algo.Market.Weight == 0 && math.Abs(quantityToOrder) > math.Abs(algo.Market.QuoteAsset.Quantity) {
			log.Println("Don't over order to go neutral")
			algo.ShouldHaveQuantity = 0
			quantityToOrder = -algo.Market.QuoteAsset.Quantity
		}

		log.Println("Can Buy", canBuy(algo), "ShouldHaveQuantity", algo.ShouldHaveQuantity, "side", side, "quantityToOrder", quantityToOrder, "leverage", algo.Market.Leverage, "leverage target", algo.LeverageTarget)

		if math.Abs(quantityToOrder) > 0 {
			orderSide = math.Copysign(1, quantityToOrder)
			if side == 1 && orderSide == 1 {
				algo.Market.BuyOrders = OrderArray{
					Quantity: []float64{math.Abs(quantityToOrder)},
					Price:    []float64{price - algo.Market.TickSize},
				}
				algo.Market.SellOrders = OrderArray{}
			} else if side == -1 && orderSide == -1 {
				algo.Market.SellOrders = OrderArray{
					Quantity: []float64{math.Abs(quantityToOrder)},
					Price:    []float64{price + algo.Market.TickSize},
				}
				algo.Market.BuyOrders = OrderArray{}
			} else {
				algo.Market.BuyOrders = OrderArray{}
				algo.Market.SellOrders = OrderArray{}
			}
		} else {
			algo.Market.BuyOrders = OrderArray{}
			algo.Market.SellOrders = OrderArray{}
		}

	} else {
		if algo.Market.Futures {
			algo.Market.BuyOrders.Quantity = utils.MulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying * algo.Market.Price.Close))
			algo.Market.SellOrders.Quantity = utils.MulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling * algo.Market.Price.Close))
		} else {
			algo.Market.BuyOrders.Quantity = utils.MulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying / algo.Market.Price.Close))
			algo.Market.SellOrders.Quantity = utils.MulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling / algo.Market.Price.Close))
		}
	}
}

func placeOrdersOnBook(algo *Algo, ex iex.IExchange, openOrders []iex.Order) {

	// For now. Should be parameterized
	qtyTolerance := 1.0
	priceTolerance := 1.0

	var newBids []iex.Order
	var newAsks []iex.Order

	createBid := func(i int, qty float64) {
		orderPrice := algo.Market.BuyOrders.Price[i]
		quantity := utils.ToFixed(qty, algo.Market.QuantityPrecision)
		quantity = utils.RoundToNearest(qty, float64(algo.Market.QuantityTickSize))
		order := iex.Order{
			Market:   algo.Market.BaseAsset.Symbol,
			Currency: algo.Market.QuoteAsset.Symbol,
			Amount:   quantity,
			Rate:     utils.ToFixed(orderPrice, algo.Market.PricePrecision),
			Type:     "Limit",
			Side:     "Buy",
		}
		newBids = append(newBids, order)
	}

	createAsk := func(i int, qty float64) {
		orderPrice := algo.Market.SellOrders.Price[i]
		quantity := utils.ToFixed(qty, algo.Market.QuantityPrecision)
		quantity = utils.RoundToNearest(qty, float64(algo.Market.QuantityTickSize))
		order := iex.Order{
			Market:   algo.Market.BaseAsset.Symbol,
			Currency: algo.Market.QuoteAsset.Symbol,
			Amount:   quantity,
			Rate:     utils.ToFixed(orderPrice, algo.Market.PricePrecision),
			Type:     "Limit",
			Side:     "Sell",
		}
		newAsks = append(newAsks, order)
	}

	cancel := func(order iex.Order) {
		// log.Println("Trying to cancel", order.OrderID)
		log.Printf("Trying to cancel order %v\n", order.OrderID)
		err := ex.CancelOrder(iex.CancelOrderF{
			Market: order.Symbol,
			Uuid:   order.OrderID,
		})
		if err != nil {
			log.Fatal(err)
		}
		// TODO when live this should be honored as not to spam the api
		// time.Sleep(50 * time.Millisecond)
	}

	place := func(order iex.Order) {
		log.Println("Trying to place order for", order.Market, "Quantity", order.Amount, "Price", order.Rate)
		_, err := ex.PlaceOrder(order)
		if err != nil {
			log.Fatal(err)
		}
		// TODO when live this should be honored as not to spam the api
		// time.Sleep(50 * time.Millisecond)
	}

	if len(algo.Market.SellOrders.Price) == 0 && len(algo.Market.BuyOrders.Price) == 0 && len(openOrders) > 0 {
		for _, order := range openOrders {
			if order.Market == algo.Market.Symbol {
				cancel(order)
			}
		}
	}

	totalQty := 0.0
	for i, qty := range algo.Market.BuyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty >= algo.Market.MinimumOrderSize {
			createBid(i, totalQty)
			totalQty = 0.0
		}
	}

	if totalQty > 0.0 && totalQty >= algo.Market.MinimumOrderSize {
		index := len(algo.Market.BuyOrders.Quantity) - 1
		createBid(index, totalQty)
	}

	totalQty = 0.0
	for i, qty := range algo.Market.SellOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty >= algo.Market.MinimumOrderSize {
			createAsk(i, totalQty)
			totalQty = 0.0
		}
	}

	if totalQty > 0.0 && totalQty >= algo.Market.MinimumOrderSize {
		index := len(algo.Market.SellOrders.Quantity) - 1
		createAsk(index, totalQty)
	}

	//Parse option orders
	for _, option := range algo.Market.OptionContracts {
		for _, order := range openOrders {
			if order.Market == option.Symbol {
				cancel(order)
			}
		}

		for i, qty := range option.BuyOrders.Quantity {
			// fmt.Printf("Parsing order for option %v: price %v qty %v\n", option.OptionTheo.String(), i, qty)
			if qty >= algo.Market.OptionMinOrderSize {
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
					Currency: algo.Market.QuoteAsset.Symbol,
					Amount:   utils.ToFixed(qty, 1), //float64(int(totalQty)),
					Rate:     utils.ToFixed(orderPrice, algo.Market.PricePrecision),
					Type:     orderType,
					Side:     "Buy",
				}
				newBids = append(newBids, order)
			}
		}

		for i, qty := range option.SellOrders.Quantity {
			// fmt.Printf("Parsing order for option %v: price %v qty %v\n", option.OptionTheo.String(), i, qty)
			if qty >= algo.Market.OptionMinOrderSize {
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
					Currency: algo.Market.QuoteAsset.Symbol,
					Amount:   utils.ToFixed(qty, 1), //float64(int(totalQty)),
					Rate:     utils.ToFixed(orderPrice, algo.Market.PricePrecision),
					Type:     orderType,
					Side:     "Sell",
				}
				// fmt.Printf("New ask from OptionContracts: %v\n", order)
				newAsks = append(newAsks, order)
			}
		}
	}

	// log.Println("Orders: Asks", newAsks, "Bids", newBids)
	// Get open buys, buys, open sells, sells, with matches filtered out
	var openBids []iex.Order
	var openAsks []iex.Order

	for _, order := range openOrders {
		if strings.ToLower(order.Side) == "buy" {
			openBids = append(openBids, order)
		} else if strings.ToLower(order.Side) == "sell" {
			openAsks = append(openAsks, order)
			// fmt.Printf("New ask from OpenOrders: %v\n", order)
		}
	}

	// Make a local sifting function
	siftMatches := func(open []iex.Order, new []iex.Order) ([]iex.Order, []iex.Order) {
		openfound := make([]bool, len(open))
		newfound := make([]bool, len(new))
		/*
			Not 100% efficient, but it's simple and predictable (more hardware friendly,
			which will likely make it more efficient). O(kn) time, but k and n should both
			be pretty small anyway.
		*/
		for i, op := range open {
			for j, nw := range new {
				if (deltaFloat(op.Rate, nw.Rate, priceTolerance)) && (deltaFloat(op.Amount, nw.Amount, qtyTolerance)) {
					openfound[i] = true
					newfound[j] = true
				}
			}
		}

		var retOpen []iex.Order
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
		return openBids[a].Rate > openBids[b].Rate
	})
	sort.Slice(openAsks, func(a, b int) bool {
		return openAsks[a].Rate < openAsks[b].Rate
	})

	bidIndex := 0
	askIndex := 0
	buyCont := len(newBids) != 0
	sellCont := len(newAsks) != 0

	// fmt.Printf("Num buys %v, num sells %v\n", len(newBids), len(newAsks))

	for buyCont || sellCont {
		if buyCont && sellCont {
			buyDiff := math.Abs(newBids[bidIndex].Rate - algo.Market.Price.Close)
			sellDiff := math.Abs(newAsks[askIndex].Rate - algo.Market.Price.Close)
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
			// finish the rest of the orders

			if algo.Market.BulkCancelSupported {
				cancelStr := ""
				for i := askIndex; i < len(openAsks); i++ {
					cancelStr += openAsks[i].OrderID + ","
				}

				for i := bidIndex; i < len(openBids); i++ {
					cancelStr += openBids[i].OrderID + ","
				}

				cancelStr = strings.TrimSuffix(cancelStr, ",")
				if len(cancelStr) > 0 {
					err := ex.CancelOrder(iex.CancelOrderF{
						Market: algo.Market.Symbol,
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

func updateLocalOrders(algo *Algo, oldOrders []iex.Order, newOrders []iex.Order, isTest bool) []iex.Order {
	var updatedOrders []iex.Order
	for _, oldOrder := range oldOrders {
		found := false
		for _, newOrder := range newOrders {
			if newOrder.OrderID == oldOrder.OrderID {
				found = true
				if newOrder.OrdStatus == orderStatus.Cancelled || newOrder.OrdStatus == orderStatus.Rejected {
					logger.Debug(newOrder.OrdStatus, oldOrder.OrderID)
				} else if newOrder.OrdStatus == orderStatus.Filled {
					newOrder.Rate = oldOrder.Rate
					newOrder.Side = oldOrder.Side
					if !isTest {
						logFilledTrade(algo, newOrder)
					}
				} else {
					updatedOrders = append(updatedOrders, newOrder)
				}
			}
		}
		if !found {
			if oldOrder.OrdStatus == orderStatus.Cancelled || oldOrder.OrdStatus == orderStatus.Filled || oldOrder.OrdStatus == orderStatus.Rejected {
				logger.Debug(oldOrder.OrdStatus, oldOrder.OrderID)
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
				logger.Debug(newOrder.OrdStatus, newOrder.OrderID)
			} else {
				logger.Debug("New Order", newOrder.OrdStatus, newOrder.OrderID)
				if !isTest {
					logTrade(algo, newOrder)
				}
				updatedOrders = append(updatedOrders, newOrder)
			}
		}
	}

	return updatedOrders
}
