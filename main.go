 //export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
 //go install GoMarketMaker && GoMarketMaker

package main

import (
	"context"
    "fmt"
	"math"
	"log"
	"time"
	"strings"
	"encoding/json"

    "GoMarketMaker/models"

	//Restful
	"github.com/zmxv/bitmexgo"

	//Websocket
	"github.com/sumorf/bitmex-api"
	"github.com/sumorf/bitmex-api/swagger"
)

var settings models.Config

func main() {
	settings = loadConfiguration("dev/mm/testnet", true)
	fireDB := setupFirebase()
	averageCost := 0.0
	quantity := 0.0
	price := 0.0

	var orders []*swagger.Order
	var b *bitmex.BitMEX
	var auth context.Context
	var client *bitmexgo.APIClient

	if settings.TestNet {
		b = bitmex.New(bitmex.HostTestnet, settings.ApiKey, settings.ApiSecret)
		auth = bitmexgo.NewAPIKeyContext(settings.ApiKey, settings.ApiSecret)
		client = bitmexgo.NewAPIClient(bitmexgo.NewTestnetConfiguration())
	} else {
		b = bitmex.New(bitmex.HostReal, settings.ApiKey, settings.ApiSecret)
		auth = bitmexgo.NewAPIKeyContext(settings.ApiKey, settings.ApiSecret)
		client = bitmexgo.NewAPIClient(bitmexgo.NewConfiguration())
	}

	subscribeInfos := []bitmex.SubscribeInfo{
		{Op: bitmex.BitmexWSOrder, Param: settings.Symbol},
		{Op: bitmex.BitmexWSPosition, Param: settings.Symbol},
		{Op: bitmex.BitmexWSTradeBin1m, Param: settings.Symbol},
	}
	
	err := b.Subscribe(subscribeInfos)
	if err != nil {
		log.Fatal(err)
	}

	b.On(bitmex.BitmexWSOrder, func(newOrders []*swagger.Order, action string) {
		orders = updateLocalOrders(orders, newOrders)
	}).On(bitmex.BitmexWSPosition, func(positions []*swagger.Position, action string) {
		position := positions[0]
		quantity = float64(position.CurrentQty)
		if math.Abs(quantity) > 0 && position.AvgCostPrice > 0 {
			averageCost = position.AvgCostPrice
		} else if position.CurrentQty == 0 {
			averageCost = 0
		}
		log.Println("AvgCostPrice", averageCost, "Quantity", quantity)
	}).On(bitmex.BitmexWSTradeBin1m, func(bins []*swagger.TradeBin, action string) {
		for _, bin := range bins {
			log.Println(bin.Close)
			price = bin.Close
			toCreate, toAmend, toCancel := placeOrdersOnBook(price, averageCost, quantity, orders)

			// log.Println(len(newOrders), "New Orders")
			// Cancel first?
			// Should consider cancel/create in 10 order blocks so cancel 10 then create the 10 to replace
			cancelOrders(auth, client, toCancel, 0)
			createOrders(auth, client, toCreate, 0)
			amendOrders(auth, client, toAmend, 0)

			updateAlgo(fireDB, "mm")
		}
	})

	b.StartWS()

	forever := make(chan bool)
	<-forever
}

func createOrders(auth context.Context, client *bitmexgo.APIClient, orders []models.Order, retry int32) {
	log.Println("Create ->", len(orders))
	if len(orders) > 0 {
		orderString := createJsonOrderString(orders)
		var orderParams bitmexgo.OrderNewBulkOpts
		orderParams.Orders.Set(orderString)
		_, res, err := client.OrderApi.OrderNewBulk(auth, &orderParams)
		if res.StatusCode != 200 || err != nil {
			if retry <= settings.MaxRetries {
				log.Println(res.StatusCode, "Retrying...")
				time.Sleep(1 * time.Second)
				createOrders(auth, client, orders, retry+1)
			}
		}
	}
}

func amendOrders(auth context.Context, client *bitmexgo.APIClient, orders []models.Order, retry int32) {
	log.Println("Amend ->",  len(orders))
	if len(orders) > 0 {
		orderString := createJsonOrderString(orders)
		var amendParams bitmexgo.OrderAmendBulkOpts
		amendParams.Orders.Set(orderString)
		_, res, err := client.OrderApi.OrderAmendBulk(auth, &amendParams)
		if res.StatusCode != 200 || err != nil {
			if retry <= settings.MaxRetries {
				log.Println(res.StatusCode, "Retrying...")
				time.Sleep(1 * time.Second)
				amendOrders(auth, client, orders, retry+1)
			}
		}
		
	}
}

func cancelOrders(auth context.Context, client *bitmexgo.APIClient, orders []string, retry int32) {
	log.Println("Cancel ->", len(orders))
	if len(orders) > 0 {
		orderString := createCancelOrderString(orders)
		var cancelParams bitmexgo.OrderCancelOpts
		cancelParams.OrderID.Set(orderString)
		// log.Println(orderString)
		_, res, err := client.OrderApi.OrderCancel(auth, &cancelParams)
		if res.StatusCode != 200 || err != nil {
			if retry <= settings.MaxRetries {
				log.Println(res.StatusCode, "Retrying...")
				time.Sleep(1 * time.Second)
				cancelOrders(auth, client, orders, retry+1)
			}
		}
	}
}


func createCancelOrderString(ids []string) string {
	orderString := strings.Join(ids,",")
	return orderString
}

func createJsonOrderString(orders []models.Order) string {
	var jsonOrders []string
	for _, o := range orders {
		jsonOrder, err := json.Marshal(o)
		if err != nil {
			log.Println(err)
			return ""
		}
		// log.Println(string(jsonOrder))
		jsonOrders = append(jsonOrders, string(jsonOrder))
	}
	orderString := "[" + strings.Join(jsonOrders,",") + "]"
	return orderString
}

func createOrderArrays(price float64, averageCost float64, quantity float64) (models.OrderArray, models.OrderArray) {
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


func placeOrdersOnBook(price float64, averageCost float64, quantity float64, currentOrders []*swagger.Order) ([]models.Order, []models.Order, []string) {
	var orders []models.Order

	buyOrders, sellOrders := createOrderArrays(price, averageCost, quantity)

	totalQty := 0.0
	for i, qty := range buyOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > 25 {
			orderPrice := buyOrders.Price[i]
			order := createLimitOrder(settings.Symbol, int32(totalQty), orderPrice, "Buy")
			orders = append(orders, order)
			totalQty = 0.0
		}
	}

	totalQty = 0.0
	for i, qty := range sellOrders.Quantity {
		totalQty = totalQty + qty
		if totalQty > 25 {
			orderPrice := sellOrders.Price[i]
			order := createLimitOrder(settings.Symbol, int32(totalQty), orderPrice, "Sell")
			orders = append(orders, order)
			totalQty = 0.0
		}
	}

	var toCreate []models.Order 
	var toAmend []models.Order
	var orderToPlace []string

	for _, newOrder := range orders {
		if newOrder.OrdType != "Market" {
			orderFound := false
			for _, oldOrder := range currentOrders {
				if !orderFound && strings.Contains(oldOrder.ClOrdID, newOrder.ClOrdID) {
					newOrder.OrigClOrdID = oldOrder.ClOrdID
					order := newOrder
					found := false
					for _, ord := range toAmend {
						if ord.ClOrdID == newOrder.ClOrdID || ord.OrigClOrdID == newOrder.OrigClOrdID {
							found = true
							break
						}
					}
					if !found {
						if !strings.Contains(newOrder.ExecInst, "Close") {
							order.OrderQty = newOrder.OrderQty //+ old_order['orderQty']
						} else {
							order.ExecInst = "Close"
						}
						orderFound = true
						// log.Println("Found order", newOrder.ClOrdID)
						// Only Ammend if qty changes
						if order.OrderQty != int32(oldOrder.OrderQty) {
							now := time.Now().Unix() - 1524872844
							order.ClOrdID = fmt.Sprintf("%s-%d", order.ClOrdID, now)
							toAmend = append(toAmend, order)
						}
						orderToPlace = append(orderToPlace, order.ClOrdID)
					}
				}
			
			}
			if !orderFound {
				now := time.Now().Unix() - 1524872844
				newOrder.ClOrdID = fmt.Sprintf("%s-%d", newOrder.ClOrdID, now)
				toCreate = append(toCreate, newOrder)
				// log.Println("Found order", newOrder.ClOrdID)
				orderToPlace = append(orderToPlace, newOrder.ClOrdID)
			}

		}
	}

	var toCancel []string
	for _, oldOrder := range currentOrders {
		found := false
		for _, newOrder := range orderToPlace {
			ordID := strings.Split(oldOrder.ClOrdID, "-")[0]
			if strings.Contains(newOrder, ordID)  {
				// log.Println("Dont Cancel", ordID, newOrder)
				found = true
				break
			}
		}
		if !found {
			toCancel = append(toCancel, oldOrder.OrderID)
		}
	}

	return toCreate, toAmend, toCancel
}



func createLimitOrder(symbol string, amount int32, price float64, side string) models.Order {
	// price = toNearest(price, coin.tick_size)
	orderId := fmt.Sprintf("%.1f_limit", price)
	order := models.Order{
		Symbol: symbol,
		ClOrdID: orderId,
		OrdType: "Limit",
		Price: price,
		OrderQty: amount,
		Side: side,
		ExecInst: "ParticipateDoNotInitiate",
	}

	return order
}


func updateLocalOrders(oldOrders []*swagger.Order, newOrders []*swagger.Order) ([]*swagger.Order) {
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

func getCostAverage(pricesFilled []float64, ordersFilled []float64) (float64, float64) {
	// print(len(prices), len(orders), len(index_arr[0]))
	percentageFilled := sumArr(ordersFilled)
	if percentageFilled > 0 {
		normalizer := 1/percentageFilled
		norm := mulArr(ordersFilled, normalizer)
		costAverage := mulArrs(pricesFilled, norm)
		return sumArr(costAverage), percentageFilled
	} else {
		return 0.0, 0.0
	}
}