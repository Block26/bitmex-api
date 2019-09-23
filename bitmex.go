package main

import (
	"log"
	"math"

	"github.com/block26/TheAlgoV2/settings"
	"github.com/block26/exchanges/bitmex"
	"github.com/block26/exchanges/bitmex/swagger"
)

var config settings.Config

func connect(settingsFile string, secret bool) {
	config = loadConfiguration(settingsFile, secret)
	// settings = loadConfiguration("dev/mm/testnet", true)
	fireDB := setupFirebase()
	averageCost := 0.0
	quantity := 0.0
	price := 0.0
	balance := 0.0

	var orders []*swagger.Order
	var b *bitmex.BitMEX

	if config.TestNet {
		b = bitmex.New(bitmex.HostTestnet, config.APIKey, config.APISecret)
	} else {
		b = bitmex.New(bitmex.HostReal, config.APIKey, config.APISecret)
	}

	subscribeInfos := []bitmex.SubscribeInfo{
		{Op: bitmex.BitmexWSOrder, Param: config.Symbol},
		{Op: bitmex.BitmexWSPosition, Param: config.Symbol},
		{Op: bitmex.BitmexWSQuoteBin1m, Param: config.Symbol},
		{Op: bitmex.BitmexWSWallet},
	}

	err := b.Subscribe(subscribeInfos)
	if err != nil {
		log.Fatal(err)
	}

	b.On(bitmex.BitmexWSWallet, func(wallet []*swagger.Wallet, action string) {
		balance = float64(wallet[len(wallet)-1].Amount)
		log.Println("balance", balance)
	}).On(bitmex.BitmexWSOrder, func(newOrders []*swagger.Order, action string) {
		orders = bitmex.UpdateLocalOrders(orders, newOrders)
	}).On(bitmex.BitmexWSPosition, func(positions []*swagger.Position, action string) {
		position := positions[0]
		quantity = float64(position.CurrentQty)
		if math.Abs(quantity) > 0 && position.AvgCostPrice > 0 {
			averageCost = position.AvgCostPrice
		} else if position.CurrentQty == 0 {
			averageCost = 0
		}
		log.Println("AvgCostPrice", averageCost, "Quantity", quantity)
	}).On(bitmex.BitmexWSQuoteBin1m, func(bins []*swagger.Quote, action string) {
		for _, bin := range bins {
			log.Println(bin.BidPrice)
			price = bin.BidPrice
			buyOrders, sellOrders := rebalance(price, averageCost, quantity)
			b.PlaceOrdersOnBook(config.Symbol, buyOrders, sellOrders, orders)
			updateAlgo(fireDB, "mm")
		}
	})

	b.StartWS()

	forever := make(chan bool)
	<-forever
}
