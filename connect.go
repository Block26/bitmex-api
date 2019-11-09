package algo

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	. "gopkg.in/src-d/go-git.v4/_examples"
)

func Connect(settingsFile string, secret bool, algo Algo, rebalance func(float64, *Algo)) {
	config = loadConfiguration(settingsFile, secret)

	// We instantiate a new repository targeting the given path (the .git folder)
	// r, err := git.PlainOpen(".")
	// CheckIfError(err)
	// ... retrieving the HEAD reference
	// ref, err := r.Head()
	commitHash = "test" //ref.Hash().String()
	// CheckIfError(err)

	exchangeVars := iex.ExchangeConf{
		Exchange:       algo.Asset.Exchange,
		ServerUrl:      algo.Asset.ExchangeURL,
		ApiSecret:      config.APISecret,
		ApiKey:         config.APIKey,
		AccountID:      "test",
		OutputResponse: false,
	}

	base_currency := "USD"
	quote_currency := "XBT"

	ex, err := tradeapi.New(exchangeVars)
	if err != nil {
		fmt.Println(err)
	}

	//Get base and quote balances
	// baseCurrencyBalance, err := ex.GetBalance(base_currency)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Printf("base_currency balance: %+v \n", baseCurrencyBalance.Available)

	// algo.Asset.BaseBalance = baseCurrencyBalance.Available

	// quoteCurrencyBalance, err := ex.GetBalance(quote_currency)
	// if err != nil {
	// 	fmt.Println(err)
	// }
	// fmt.Printf("quote_currency balance: %+v \n", quoteCurrencyBalance.Available)
	// algo.Asset.Quantity = quoteCurrencyBalance.Available

	// mkt, err := ex.GetMarketSummary(quote_currency, base_currency)
	// fmt.Printf("markets: %+v \n", mkt)

	// channels to subscribe to
	symbol := strings.ToLower(quote_currency + base_currency)

	subscribeInfos := []iex.WSSubscribeInfo{
		{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol},
		{Name: iex.WS_WALLET, Symbol: symbol},
		{Name: iex.WS_ORDER, Symbol: symbol},
	}

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		TradeBinChan: make(chan []iex.TradeBin, 2),
		WalletChan:   make(chan *iex.WSWallet, 2),
		OrderChan:    make(chan []iex.WSOrder, 2),
	}

	// LogStatus(&algo)
	// Start the websocket.
	err = ex.StartWS(&iex.WsConfig{Host: "testnet.bitmex.com", //"stream.binance.us:9443",
		Streams:   subscribeInfos,
		Channels:  channels,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	var localOrders []iex.WSOrder

	for {
		select {
		case trade := <-channels.TradeBinChan:
			log.Println("Trade Update:", trade)
			algo.Asset.Price = trade[0].Close
			rebalance(trade[0].Close, &algo)
			algo.PlaceOrdersOnBook(localOrders)
		case newOrders := <-channels.OrderChan:
			localOrders = UpdateLocalOrders(localOrders, newOrders)
		case update := <-channels.WalletChan:
			walletAmount := float64(update.Balance[0].Amount)
			if walletAmount > 0 {
				algo.Asset.BaseBalance = walletAmount * 0.00000001
				fmt.Printf("BaseBalance: %+v \n", algo.Asset.BaseBalance)
			}
		}
	}

	// algo.BuyOrders.Quantity = mulArr(algo.BuyOrders.Quantity, (algo.Asset.Buying * mkt.Last))
	// algo.SellOrders.Quantity = mulArr(algo.SellOrders.Quantity, (algo.Asset.Selling * mkt.Last))
	log.Println("algo.Asset.BaseBalance", algo.Asset.BaseBalance)
	log.Println("Total Buy BTC", (algo.Asset.Buying))
	// log.Println("Total Buy USD", (algo.Asset.Buying * mkt.Last))
	log.Println("Total Sell BTC", (algo.Asset.Selling))
	// log.Println("Total Sell USD", (algo.Asset.Selling * mkt.Last))
	// log.Println("Local order length", len(orders))
	log.Println("New order length", len(algo.BuyOrders.Quantity), len(algo.SellOrders.Quantity))
	// log.Println("Buys", algo.BuyOrders.Quantity)
	// log.Println("Sells", algo.SellOrders.Quantity)
	// log.Println("New order length", len(algo.BuyOrders.Price), len(algo.SellOrders.Price))
	// orders, err := ex.OpenOrders(iex.OpenOrderF{Market: quote_currency + base_currency})
	// toCreate, toCancel := algo.PlaceOrdersOnBook(orders)
	// log.Println(toCreate)
	// log.Println(toCancel)
	algo.logState("")
	// updateAlgo(fireDB, "mm")
}

func LogStatus(algo *Algo) {
	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://ec2-54-219-145-3.us-west-1.compute.amazonaws.com:8086",
		Username: "russell",
		Password: "KNW(12nAS921D",
	})
	CheckIfError(err)

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": algo.Name, "commit_hash": commitHash}

	fields := structs.Map(algo.Asset)

	pt, err := client.NewPoint(
		"asset",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	for index := 0; index < len(algo.BuyOrders.Quantity); index++ {

		fields = map[string]interface{}{
			fmt.Sprintf("%0.2f", algo.BuyOrders.Price[index]): algo.BuyOrders.Quantity[index],
		}

		pt, err = client.NewPoint(
			"buy_orders",
			tags,
			fields,
			time.Now(),
		)
		bp.AddPoint(pt)
	}

	for index := 0; index < len(algo.SellOrders.Quantity); index++ {
		fields = map[string]interface{}{
			fmt.Sprintf("%0.2f", algo.SellOrders.Price[index]): algo.SellOrders.Quantity[index],
		}
		pt, err = client.NewPoint(
			"sell_orders",
			tags,
			fields,
			time.Now(),
		)
		bp.AddPoint(pt)
	}

	if algo.State != nil {
		pt, err := client.NewPoint(
			"state",
			tags,
			algo.State,
			time.Now(),
		)
		CheckIfError(err)
		bp.AddPoint(pt)
	}

	err = client.Client.Write(influx, bp)
	CheckIfError(err)
	influx.Close()
}
