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
	algo := Algo{
		Asset: Asset{
			BaseBalance: 1.0,
			Quantity:    220.0,
			AverageCost: 0.0,
			MaxOrders:   10,
			MaxLeverage: 0.2,
		},
		EntrySpread:     0.05,
		EntryConfidence: 1,
		ExitSpread:      0.03,
		ExitConfidence:  0.1,
		Liquidity:       0.1,
	}
	// settings = loadConfiguration("dev/mm/testnet", true)
	fireDB := setupFirebase()

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
		algo.Asset.BaseBalance = float64(wallet[len(wallet)-1].Amount)
		log.Println("algo.Asset.BaseBalance", algo.Asset.BaseBalance)
	}).On(bitmex.BitmexWSOrder, func(newOrders []*swagger.Order, action string) {
		orders = bitmex.UpdateLocalOrders(orders, newOrders)
	}).On(bitmex.BitmexWSPosition, func(positions []*swagger.Position, action string) {
		position := positions[0]
		algo.Asset.Quantity = float64(position.CurrentQty)
		if math.Abs(algo.Asset.Quantity) > 0 && position.AvgCostPrice > 0 {
			algo.Asset.AverageCost = position.AvgCostPrice
		} else if position.CurrentQty == 0 {
			algo.Asset.AverageCost = 0
		}
		log.Println("AvgCostPrice", algo.Asset.AverageCost, "Quantity", algo.Asset.Quantity)
	}).On(bitmex.BitmexWSQuoteBin1m, func(bins []*swagger.Quote, action string) {
		for _, bin := range bins {
			log.Println(bin.BidPrice)
			algo.rebalance(bin.BidPrice)
			b.PlaceOrdersOnBook(config.Symbol, algo.BuyOrders, algo.SellOrders, orders)
			updateAlgo(fireDB, "mm")
		}
	})

	b.StartWS()

	forever := make(chan bool)
	<-forever
}
