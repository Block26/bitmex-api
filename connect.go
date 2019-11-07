package algo

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/influxdata/influxdb-client-go"

	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"

	"gopkg.in/src-d/go-git.v4"
	. "gopkg.in/src-d/go-git.v4/_examples"
)

func Connect(settingsFile string, secret bool, algo Algo, rebalance func(float64, *Algo)) {
	config = loadConfiguration(settingsFile, secret)

	influx, err := influxdb.New("https://us-west-2-1.aws.cloud2.influxdata.com",
		"xskhvPlukzR2jXsKO2jbfcW_g6ekxpMKrfTx5Ui400iKjeG-bTQTeQf_fgjT_dH0jYQbls0b_F_sDgITQVn4hA==",
		influxdb.WithHTTPClient(http.DefaultClient))

	if err != nil {
		log.Fatal(err)
	}

	// We instantiate a new repository targeting the given path (the .git folder)
	r, err := git.PlainOpen(".")
	CheckIfError(err)
	// ... retrieving the HEAD reference
	ref, err := r.Head()
	commitHash = ref.Hash().String()
	CheckIfError(err)

	exchangeVars := iex.ExchangeConf{
		Exchange:       "binance",
		ApiSecret:      config.APISecret,
		ApiKey:         config.APIKey,
		AccountID:      "test",
		OutputResponse: false,
	}

	base_currency := "USDT"
	quote_currency := "BTC"

	ex, err := tradeapi.New(exchangeVars)
	if err != nil {
		fmt.Println(err)
	}

	//Get base and quote balances
	baseCurrencyBalance, err := ex.GetBalance(base_currency)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("base_currency balance: %+v \n", baseCurrencyBalance.Available)

	algo.Asset.BaseBalance = baseCurrencyBalance.Available

	quoteCurrencyBalance, err := ex.GetBalance(quote_currency)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("quote_currency balance: %+v \n", quoteCurrencyBalance.Available)
	algo.Asset.Quantity = quoteCurrencyBalance.Available

	mkt, err := ex.GetMarketSummary(quote_currency, base_currency)
	// fmt.Printf("markets: %+v \n", mkt)
	algo.Asset.Price = mkt.Last
	fmt.Printf("algo.Asset.Price: %+v \n", mkt.Last)

	// channels to subscribe to
	subscribeInfos := []iex.WSSubscribeInfo{
		{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: strings.ToLower(quote_currency + base_currency)},
	}

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		TradeBinChan: make(chan []iex.TradeBin, 2),
	}

	LogStatus(influx, &algo)
	// Start the websocket.
	err = ex.StartWS(&iex.WsConfig{Host: "", Streams: subscribeInfos, Channels: channels})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	for {
		select {
		case update := <-channels.TradeBinChan:
			fmt.Printf("Trade Update: %+v \n", update)
		}
	}

	rebalance(mkt.Last, &algo)
	// algo.BuyOrders.Quantity = mulArr(algo.BuyOrders.Quantity, (algo.Asset.Buying * mkt.Last))
	// algo.SellOrders.Quantity = mulArr(algo.SellOrders.Quantity, (algo.Asset.Selling * mkt.Last))
	log.Println("algo.Asset.BaseBalance", algo.Asset.BaseBalance)
	log.Println("Total Buy BTC", (algo.Asset.Buying))
	log.Println("Total Buy USD", (algo.Asset.Buying * mkt.Last))
	log.Println("Total Sell BTC", (algo.Asset.Selling))
	log.Println("Total Sell USD", (algo.Asset.Selling * mkt.Last))
	// log.Println("Local order length", len(orders))
	log.Println("New order length", len(algo.BuyOrders.Quantity), len(algo.SellOrders.Quantity))
	// log.Println("Buys", algo.BuyOrders.Quantity)
	// log.Println("Sells", algo.SellOrders.Quantity)
	// log.Println("New order length", len(algo.BuyOrders.Price), len(algo.SellOrders.Price))
	orders, err := ex.OpenOrders(iex.OpenOrderF{Market: quote_currency + base_currency})
	toCreate, toCancel := algo.PlaceOrdersOnBook(orders)
	log.Println(toCreate)
	log.Println(toCancel)
	algo.logState("")
	influx.Close()
	// updateAlgo(fireDB, "mm")
}

func LogStatus(influx *influxdb.Client, algo *Algo) {

	myMetrics := []influxdb.Metric{
		influxdb.NewRowMetric(structs.Map(algo.Asset), "asset", map[string]string{"algo_name": algo.Name, "commit_hash": commitHash}, time.Now()),
	}

	if algo.State != nil {
		myMetrics = append(myMetrics, influxdb.NewRowMetric(algo.State, "state", map[string]string{"algo_name": algo.Name, "commit_hash": commitHash}, time.Now()))
	}

	// The actual write..., this method can be called concurrently.
	if _, err := influx.Write(context.Background(), "algos", "Tantra Labs", myMetrics...); err != nil {
		log.Fatal(err) // as above use your own error handling here.
	}
}
