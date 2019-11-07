package algo

import (
	"log"
	"math"
	"net/http"

	"github.com/influxdata/influxdb-client-go"
	"github.com/tantralabs/TheAlgoV2/data"
	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/TheAlgoV2/settings"
	"github.com/tantralabs/exchanges/bitmex"
	"github.com/tantralabs/exchanges/bitmex/swagger"
	"gopkg.in/src-d/go-git.v4"
	. "gopkg.in/src-d/go-git.v4/_examples"
)

var config settings.Config

func ConnectToBitmex(settingsFile string, secret bool, algo Algo, rebalance func(float64, *Algo), setupData func(*[]models.Bar, *Algo)) {

	if algo.Name == "" {
		log.Fatal("Name not set")
	}

	if algo.Asset.Symbol == "" {
		log.Fatal("Asset Symbol not set")
	}

	config = loadConfiguration(settingsFile, secret)

	influx, err := influxdb.New("https://us-west-2-1.aws.cloud2.influxdata.com",
		"xskhvPlukzR2jXsKO2jbfcW_g6ekxpMKrfTx5Ui400iKjeG-bTQTeQf_fgjT_dH0jYQbls0b_F_sDgITQVn4hA==",
		influxdb.WithHTTPClient(http.DefaultClient))

	// We instantiate a new repository targeting the given path (the .git folder)
	r, err := git.PlainOpen(".")
	CheckIfError(err)
	// ... retrieving the HEAD reference
	ref, err := r.Head()
	commitHash = ref.Hash().String()
	CheckIfError(err)

	if err != nil {
		log.Fatal(err)
	}
	// settings = loadConfiguration("dev/mm/testnet", true)
	log.Println(config)
	fireDB := setupFirebase()

	var orders []*swagger.Order
	var b *bitmex.BitMEX

	localBars := data.GetData(algo.Asset.Symbol, "1m", algo.DataLength)
	log.Println(len(localBars), "downloaded")

	if config.TestNet {
		b = bitmex.New(bitmex.HostTestnet, config.APIKey, config.APISecret)
	} else {
		b = bitmex.New(bitmex.HostReal, config.APIKey, config.APISecret)
	}

	subscribeInfos := []bitmex.SubscribeInfo{
		{Op: bitmex.BitmexWSOrder, Param: algo.Asset.Symbol},
		{Op: bitmex.BitmexWSPosition, Param: algo.Asset.Symbol},
		{Op: bitmex.BitmexWSQuoteBin1m, Param: algo.Asset.Symbol},
		{Op: bitmex.BitmexWSWallet},
	}

	err = b.Subscribe(subscribeInfos)
	if err != nil {
		log.Fatal(err)
	}

	b.On(bitmex.BitmexWSWallet, func(wallet []*swagger.Wallet, action string) {
		walletAmount := float64(wallet[len(wallet)-1].Amount)
		if walletAmount > 0 {
			algo.Asset.BaseBalance = walletAmount * 0.00000001
			log.Println("algo.Asset.BaseBalance", algo.Asset.BaseBalance)
		} else {
			// TODO if it returns zero, query again after a set amount of time
			log.Println("Error with wallet amount, Wallet returned 0")
		}
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
			algo.Asset.Price = bin.BidPrice
			localBars = data.UpdateLocalBars(localBars, data.GetData("XBTUSD", "1m", 2))
			setupData(&localBars, &algo)
			rebalance(bin.BidPrice, &algo)
			algo.BuyOrders.Quantity = mulArr(algo.BuyOrders.Quantity, (algo.Asset.Buying * bin.BidPrice))
			algo.SellOrders.Quantity = mulArr(algo.SellOrders.Quantity, (algo.Asset.Selling * bin.BidPrice))
			log.Println("algo.Asset.BaseBalance", algo.Asset.BaseBalance)
			log.Println("Total Buy BTC", (algo.Asset.Buying))
			log.Println("Total Buy USD", (algo.Asset.Buying * bin.BidPrice))
			log.Println("Total Sell BTC", (algo.Asset.Selling))
			log.Println("Total Sell USD", (algo.Asset.Selling * bin.BidPrice))
			log.Println("Local order length", len(orders))
			log.Println("New order length", len(algo.BuyOrders.Quantity), len(algo.SellOrders.Quantity))
			// log.Println("Buys", algo.BuyOrders.Quantity)
			// log.Println("Sells", algo.SellOrders.Quantity)
			// log.Println("New order length", len(algo.BuyOrders.Price), len(algo.SellOrders.Price))
			b.PlaceOrdersOnBook(config.Symbol, algo.BuyOrders, algo.SellOrders, orders)
			LogStatus(influx, &algo)
			algo.logState("")
			updateAlgo(fireDB, "mm")
		}
	})

	b.StartWS()

	forever := make(chan bool)
	<-forever
}
