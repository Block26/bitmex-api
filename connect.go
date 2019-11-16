package algo

import (
	"fmt"
	"log"
	"math"
	"os"
	"strings"

	"github.com/tantralabs/TheAlgoV2/data"
	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
)

func Connect(settingsFile string, secret bool, algo Algo, rebalance func(float64, Algo) Algo, setupData func(*[]models.Bar, Algo)) {
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

	ex, err := tradeapi.New(exchangeVars)
	if err != nil {
		fmt.Println(err)
	}

	localBars := data.GetData(algo.Asset.Symbol, "1m", algo.DataLength)
	log.Println(len(localBars), "downloaded")

	// channels to subscribe to
	symbol := strings.ToLower(algo.Asset.Market + algo.Asset.Currency)
	//Ordering is important, get wallet and position first then market info
	subscribeInfos := []iex.WSSubscribeInfo{
		{Name: iex.WS_WALLET, Symbol: symbol},
		{Name: iex.WS_ORDER, Symbol: symbol},
		{Name: iex.WS_POSITION, Symbol: symbol},
		{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol},
	}

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan: make(chan []iex.WsPosition, 2),
		TradeBinChan: make(chan []iex.TradeBin, 2),
		WalletChan:   make(chan *iex.WSWallet, 2),
		OrderChan:    make(chan []iex.WSOrder, 2),
	}

	// Start the websocket.
	err = ex.StartWS(&iex.WsConfig{Host: algo.Asset.WSStream, //"testnet.bitmex.com", //"stream.binance.us:9443",
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
		case positions := <-channels.PositionChan:
			log.Println("Position Update:", positions)
			position := positions[0]
			algo.Asset.Quantity = float64(position.CurrentQty)
			if math.Abs(algo.Asset.Quantity) > 0 && position.AvgCostPrice > 0 {
				algo.Asset.AverageCost = position.AvgCostPrice
			} else if position.CurrentQty == 0 {
				algo.Asset.AverageCost = 0
			}
			log.Println("AvgCostPrice", algo.Asset.AverageCost, "Quantity", algo.Asset.Quantity)
			algo.logState()
		case trade := <-channels.TradeBinChan:
			log.Println("Trade Update:", trade)
			algo.Asset.Price = trade[0].Close
			localBars = data.UpdateLocalBars(localBars, data.GetData("XBTUSD", "1m", 2))
			log.Println("Bars", len(localBars))
			setupData(&localBars, algo)
			algo.Index = len(localBars) - 1
			algo = rebalance(trade[0].Close, algo)
			algo.BuyOrders.Quantity = mulArr(algo.BuyOrders.Quantity, (algo.Asset.Buying * algo.Asset.Price))
			algo.SellOrders.Quantity = mulArr(algo.SellOrders.Quantity, (algo.Asset.Selling * algo.Asset.Price))
			algo.PlaceOrdersOnBook(ex, localOrders)
			algo.logState()
		case newOrders := <-channels.OrderChan:
			localOrders = UpdateLocalOrders(localOrders, newOrders)
		case update := <-channels.WalletChan:
			walletAmount := float64(update.Balance[0].Balance)
			if walletAmount > 0 {
				algo.Asset.BaseBalance = walletAmount * 0.00000001
				fmt.Printf("BaseBalance: %+v \n", algo.Asset.BaseBalance)
			}
		}
	}
}
