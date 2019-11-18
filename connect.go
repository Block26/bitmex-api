package algo

import (
	"fmt"
	"log"
	"strings"

	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
)

var orderStatus iex.OrderStatus

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

	localBars := make([]models.Bar, 0) //data.GetData(algo.Market.Symbol, "1m", algo.DataLength)
	// localBars := data.GetData(algo.Market.Symbol, "1m", algo.DataLength)
	// log.Println(len(localBars), "downloaded")

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
	openOrders, err := ex.OpenOrders(iex.OpenOrderF{Market: algo.Market.BaseAsset, Currency: algo.Market.QuoteAsset})

	if err != nil {
		fmt.Println(err)
	}

	log.Println("openOrders", openOrders)
	for i := range openOrders.Bids {
		oo := openOrders.Bids[i]
		order := iex.WSOrder{
			Symbol:    oo.Market,
			Price:     oo.Price,
			OrderQty:  oo.Quantity,
			OrderID:   oo.UUID,
			OrdStatus: oo.Status,
			Side:      oo.Side,
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
			Side:      oo.Side,
		}
		localOrders = append(localOrders, order)
	}
	var emptyOrders []iex.WSOrder
	localOrders = UpdateLocalOrders(emptyOrders, localOrders)
	balances, err := ex.GetBalances()
	algo.updateAlgoBalances(balances)
	algo.logState()
	for {
		select {
		case positions := <-channels.PositionChan:
			log.Println("Position Update:", positions)
			// position := positions[0]
			// algo.Asset.Quantity = float64(position.CurrentQty)
			// if math.Abs(algo.BaseAsset.Quantity) > 0 && position.AvgCostPrice > 0 {
			// 	algo.Asset.AverageCost = position.AvgCostPrice
			// } else if position.CurrentQty == 0 {
			// 	algo.Asset.AverageCost = 0
			// }
			// log.Println("AvgCostPrice", algo.Asset.AverageCost, "Quantity", algo.Asset.Quantity)
			// algo.logState()
		case trade := <-channels.TradeBinChan:
			log.Println("Trade Update:", trade)
			algo.Market.Price = trade[0].Close
			// localBars = data.UpdateLocalBars(localBars, data.GetData("XBTUSD", "1m", 2))
			// log.Println("Bars", len(localBars))
			setupData(&localBars, algo)
			algo.Index = len(localBars) - 1
			algo = rebalance(trade[0].Close, algo)
			if algo.Market.Futures {
				algo.Market.BuyOrders.Quantity = mulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying * algo.Market.Price))
				algo.Market.SellOrders.Quantity = mulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling * algo.Market.Price))
			} else {
				log.Println("Buying", algo.Market.Buying, algo.Market.BuyOrders.Quantity)
				log.Println("Selling", algo.Market.Selling, algo.Market.SellOrders.Quantity)
				algo.Market.BuyOrders.Quantity = mulArr(algo.Market.BuyOrders.Quantity, (algo.Market.Buying / algo.Market.Price))
				algo.Market.SellOrders.Quantity = mulArr(algo.Market.SellOrders.Quantity, (algo.Market.Selling / algo.Market.Price))
				log.Println("Buying", algo.Market.Buying, algo.Market.BuyOrders.Quantity)
				log.Println("Selling", algo.Market.Selling, algo.Market.SellOrders.Quantity)
			}
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
		if balances[i].Asset == algo.BaseAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.BaseAsset.Quantity {
				algo.BaseAsset.Quantity = walletAmount
				fmt.Printf("BaseAsset: %+v \n", walletAmount)
			}
		} else if balances[i].Asset == algo.QuoteAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.QuoteAsset.Quantity {
				algo.QuoteAsset.Quantity = walletAmount
				fmt.Printf("QuoteAsset: %+v \n", walletAmount)
			}
		}
	}
}
