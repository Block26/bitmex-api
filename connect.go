package algo

import (
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/tantralabs/TheAlgoV2/data"
	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
)

var orderStatus iex.OrderStatus
var firstTrade bool

func Connect(settingsFile string, secret bool, algo Algo, rebalance func(float64, Algo) Algo, setupData func([]*models.Bar, Algo)) {
	firstTrade = false
	config = loadConfiguration(settingsFile, secret)
	fmt.Printf("Loaded config: %v\n", config)
	// We instantiate a new repository targeting the given path (the .git folder)
	// r, err := git.PlainOpen(".")
	// CheckIfError(err)
	// ... retrieving the HEAD reference
	// ref, err := r.Head()
	commitHash = "test" //ref.Hash().String()
	// CheckIfError(err)

	exchangeVars := iex.ExchangeConf{
		Exchange:       algo.Market.Exchange,
		ServerUrl:      algo.Market.ExchangeURL,
		ApiSecret:      config.APISecret,
		ApiKey:         config.APIKey,
		AccountID:      "test",
		OutputResponse: false,
	}

	ex, err := tradeapi.New(exchangeVars)
	if err != nil {
		fmt.Println(err)
	}
	orderStatus = ex.GetPotentialOrderStatus()

	// localBars := make([]*models.Bar, 0)
	localBars := data.GetData(algo.Market.Symbol, algo.DecisionInterval, algo.DataLength+1)
	log.Println(len(localBars), "downloaded")

	// channels to subscribe to
	symbol := strings.ToLower(algo.Market.Symbol)
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
	err = ex.StartWS(&iex.WsConfig{Host: algo.Market.WSStream, //"testnet.bitmex.com", //"stream.binance.us:9443",
		Streams:   subscribeInfos,
		Channels:  channels,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	if err != nil {
		fmt.Println(err)
	}

	var localOrders []iex.WSOrder

	//Setup local orders - Some exchanges don't send orders on WS connection so we prepopulate from restful api
	openOrders, err := ex.OpenOrders(iex.OpenOrderF{Market: algo.Market.BaseAsset.Symbol, Currency: algo.Market.QuoteAsset.Symbol})

	if err != nil {
		fmt.Println(err)
	}

	for i := range openOrders.Bids {
		oo := openOrders.Bids[i]
		order := iex.WSOrder{
			Symbol:    oo.Market,
			Price:     oo.Price,
			OrderQty:  oo.Quantity,
			OrderID:   oo.UUID,
			OrdStatus: oo.Status,
			// Side:      oo.Side,
		}
		localOrders = append(localOrders, order)
	}

	for i := range openOrders.Asks {
		oo := openOrders.Asks[i]
		order := iex.WSOrder{
			Symbol:    oo.Market,
			Price:     oo.Price,
			OrderQty:  oo.Quantity,
			OrderID:   oo.UUID,
			OrdStatus: oo.Status,
			// Side:      oo.Side,
		}
		localOrders = append(localOrders, order)
	}
	var emptyOrders []iex.WSOrder
	localOrders = UpdateLocalOrders(emptyOrders, localOrders)
	balances, err := ex.GetBalances()
	algo.updateAlgoBalances(balances)

	for {
		select {
		case positions := <-channels.PositionChan:
			log.Println("Position Update:", positions)
			position := positions[0]
			algo.Market.QuoteAsset.Quantity = float64(position.CurrentQty)
			if math.Abs(algo.Market.QuoteAsset.Quantity) > 0 && position.AvgCostPrice > 0 {
				algo.Market.AverageCost = position.AvgCostPrice
			} else if position.CurrentQty == 0 {
				algo.Market.AverageCost = 0
			}
			log.Println("AvgCostPrice", algo.Market.AverageCost, "Quantity", algo.Market.QuoteAsset.Quantity)
			// algo.logState()
		case trade := <-channels.TradeBinChan:
			algo.updateState(trade[0], &localBars, setupData)
			algo = rebalance(trade[0].Close, algo)
			algo.setupOrders()
			algo.PlaceOrdersOnBook(ex, localOrders)
			algo.logState()
		case newOrders := <-channels.OrderChan:
			// log.Println("update channels.OrderChan")
			localOrders = UpdateLocalOrders(localOrders, newOrders)
		case update := <-channels.WalletChan:
			algo.updateAlgoBalances(update.Balance)
		}
	}
}

func (algo *Algo) updateAlgoBalances(balances []iex.WSBalance) {
	for i := range balances {
		if balances[i].Asset == algo.Market.BaseAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.BaseAsset.Quantity {
				algo.Market.BaseAsset.Quantity = walletAmount
				fmt.Printf("BaseAsset: %+v \n", walletAmount)
			}
		} else if balances[i].Asset == algo.Market.QuoteAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.QuoteAsset.Quantity {
				algo.Market.QuoteAsset.Quantity = walletAmount
				fmt.Printf("QuoteAsset: %+v \n", walletAmount)
			}
		}
	}
}

func (algo *Algo) updateState(trade iex.TradeBin, localBars *[]*models.Bar, setupData func([]*models.Bar, Algo)) {
	log.Println("Trade Update:", trade)
	algo.Market.Price = trade.Close
	data.UpdateLocalBars(localBars, data.GetData(algo.Market.Symbol, algo.DecisionInterval, 2))
	setupData(*localBars, *algo)
	algo.Index = len(*localBars) - 1
	log.Println("algo.Index", algo.Index)
	if firstTrade {
		algo.logState()
		firstTrade = false
	}
}

func (algo *Algo) setupOrders() {
	if algo.AutoOrderPlacement {

	} else {
		if algo.Market.Futures {
			algo.Market.BuyOrders.Quantity = mulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying * algo.Market.Price))
			algo.Market.SellOrders.Quantity = mulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling * algo.Market.Price))
		} else {
			algo.Market.BuyOrders.Quantity = mulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying / algo.Market.Price))
			algo.Market.SellOrders.Quantity = mulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling / algo.Market.Price))
		}
	}
}
