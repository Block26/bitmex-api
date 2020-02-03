package yantra

import (
	"log"
	"math"
	"strings"
	"time"

	. "github.com/tantralabs/models"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/database"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/logger"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"

	"github.com/jinzhu/copier"
)

var orderStatus iex.OrderStatus
var firstTrade bool
var firstPositionUpdate bool
var commitHash string
var lastTest int64
var lastWalletSync int64

// Connect is called to connect to an exchanges WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of Algo.RebalanceInterval
//
// This is intentional, look at Algo.AutoOrderPlacement to understand this paradigm.
func Connect(settingsFileName string, secret bool, algo Algo, rebalance func(Algo) Algo, setupData func([]*Bar, Algo)) {
	database.Setup("remote")
	if algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}
	firstTrade = true
	firstPositionUpdate = true
	config := utils.LoadSecret(settingsFileName, secret)
	logger.Info("Loaded config for", algo.Market.Exchange, "secret", settingsFileName)
	commitHash = time.Now().String()

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
		logger.Error(err)
	}

	orderStatus = ex.GetPotentialOrderStatus()
	logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", algo.Market.Symbol, algo.RebalanceInterval, algo.DataLength+1)
	localBars := database.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, algo.DataLength+100)
	// Set initial timestamp for algo
	algo.Timestamp = time.Unix(data.GetBars()[algo.Index].Timestamp/1000, 0).UTC()
	logger.Infof("Got local bars: %v\n", len(localBars))

	if algo.Market.Options {
		// Build theo engine
		theoEngine := te.NewTheoEngine(&algo.Market, ex, &algo.Timestamp, 60000, 86400000, false, 0, 0)
		algo.TheoEngine = &theoEngine
		logger.Infof("Built theo engine.\n")
	}

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := ex.GetBalances()
	updateAlgoBalances(&algo, balances)

	positions, _ := ex.GetPositions(algo.Market.BaseAsset.Symbol)
	updatePositions(&algo, positions)

	var localOrders []iex.Order
	orders, _ := ex.GetOpenOrders(iex.OpenOrderF{Currency: algo.Market.BaseAsset.Symbol})
	localOrders = updateLocalOrders(&algo, localOrders, orders)
	logger.Infof("%v orders found", len(localOrders))
	// SUBSCRIBE TO WEBSOCKETS

	// channels to subscribe to
	symbol := strings.ToLower(algo.Market.Symbol)
	//Ordering is important, get wallet and position first then market info
	subscribeInfos := []iex.WSSubscribeInfo{
		{Name: iex.WS_WALLET, Symbol: symbol},
		{Name: iex.WS_ORDER, Symbol: symbol},
		{Name: iex.WS_POSITION, Symbol: symbol},
		{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol, Market: iex.WSMarketType{Contract: iex.WS_SWAP}},
	}

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan: make(chan []iex.WsPosition, 2),
		TradeBinChan: make(chan []iex.TradeBin, 2),
		WalletChan:   make(chan *iex.WSWallet, 2),
		OrderChan:    make(chan []iex.Order, 2),
	}

	// Start the websocket.
	err = ex.StartWS(&iex.WsConfig{Host: algo.Market.WSStream, //"testnet.bitmex.com", //"stream.binance.us:9443",
		Streams:   subscribeInfos,
		Channels:  channels,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	if err != nil {
		logger.Error(err)
	}

	for {
		select {
		case positions := <-channels.PositionChan:
			updatePositions(&algo, positions)
		case trade := <-channels.TradeBinChan:
			updateBars(&algo, ex, trade[0])
			updateState(&algo, ex, trade[0], setupData)
			algo = rebalance(algo)
			setupOrders(&algo, trade[0].Close)
			placeOrdersOnBook(&algo, ex, localOrders)
			logState(&algo)
			runTest(&algo, setupData, rebalance)
			// updateOptionPositions(&algo,  )
			if secret {
				checkWalletHistory(&algo, ex, settingsFileName)
			}
		case newOrders := <-channels.OrderChan:
			localOrders = updateLocalOrders(&algo, localOrders, newOrders)
		case update := <-channels.WalletChan:
			updateAlgoBalances(&algo, update.Balance)
		}
	}
}

func checkWalletHistory(algo *Algo, ex iex.IExchange, settingsFileName string) {
	timeSinceLastSync := database.GetBars()[algo.Index].Timestamp - lastWalletSync
	if timeSinceLastSync > (60 * 60 * 60) {
		logger.Info("It has been", timeSinceLastSync, "seconds since the last wallet history download, fetching latest deposits and withdrawals.")
		lastWalletSync = database.GetBars()[algo.Index].Timestamp
		walletHistory, err := ex.GetWalletHistory(algo.Market.BaseAsset.Symbol)
		if err != nil {
			logger.Error("There was an error fetching the wallet history", err)
		} else {
			if len(walletHistory) > 0 {
				database.LogWalletHistory(algo, settingsFileName, walletHistory)
			}
		}
	}
}

func runTest(algo *Algo, setupData func([]*Bar, Algo), rebalance func(Algo) Algo) {
	if lastTest != database.GetBars()[algo.Index].Timestamp {
		lastTest = database.GetBars()[algo.Index].Timestamp
		testAlgo := Algo{}
		copier.Copy(&testAlgo, &algo)
		logger.Info(testAlgo.Market.BaseAsset.Quantity)
		// RESET Algo but leave base balance
		testAlgo.Market.QuoteAsset.Quantity = 0
		testAlgo.Market.Leverage = 0
		testAlgo.Market.Weight = 0
		// Override logger level to info so that we don't pollute logs with backtest state changes
		numOptions := len(algo.Market.OptionContracts)
		testAlgo = RunBacktest(data.GetBars(), testAlgo, rebalance, setupData)
		logger.Livef("Backtest added %v options\n", len(algo.Market.OptionContracts)-numOptions)
		logLiveState(&testAlgo, true)
		//TODO compare the states
	}
}

func updatePositions(algo *Algo, positions []iex.WsPosition) {
	logger.Info("Position Update:", positions)
	if len(positions) > 0 {
		for _, position := range positions {
			if position.Symbol == algo.Market.QuoteAsset.Symbol || position.Symbol == algo.Market.Symbol {
				algo.Market.QuoteAsset.Quantity = float64(position.CurrentQty)
				if math.Abs(algo.Market.QuoteAsset.Quantity) > 0 && position.AvgCostPrice > 0 {
					algo.Market.AverageCost = position.AvgCostPrice
				} else if position.CurrentQty == 0 {
					algo.Market.AverageCost = 0
				}
				logger.Info("AvgCostPrice", algo.Market.AverageCost, "Quantity", algo.Market.QuoteAsset.Quantity)
			} else if position.Symbol == algo.Market.BaseAsset.Symbol {
				algo.Market.BaseAsset.Quantity = float64(position.CurrentQty)
				logger.Info("BaseAsset updated")
			} else {
				for i := range algo.Market.OptionContracts {
					option := &algo.Market.OptionContracts[i]
					if option.Symbol == position.Symbol {
						option.Position = position.CurrentQty
						option.AverageCost = position.AvgCostPrice
						option.Profit = (option.MidMarketPrice - position.AvgCostPrice) * math.Abs(position.CurrentQty)
						logger.Infof("[%v] Updated position %v, average cost %v, profit %v\n", option.Symbol, option.Position, option.AverageCost, option.Profit)
						break
					}
				}
			}
		}
		if firstPositionUpdate {
			logState(algo)
			algo.ShouldHaveQuantity = algo.Market.QuoteAsset.Quantity
			firstPositionUpdate = false
		}
	}
	logState(algo)
}

func updateAlgoBalances(algo *Algo, balances []iex.WSBalance) {
	for i := range balances {
		if balances[i].Asset == algo.Market.BaseAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.BaseAsset.Quantity {
				algo.Market.BaseAsset.Quantity = walletAmount
				logger.Infof("BaseAsset: %+v \n", walletAmount)
			}
		} else if balances[i].Asset == algo.Market.QuoteAsset.Symbol {
			walletAmount := float64(balances[i].Balance)
			if walletAmount > 0 && walletAmount != algo.Market.QuoteAsset.Quantity {
				algo.Market.QuoteAsset.Quantity = walletAmount
				logger.Infof("QuoteAsset: %+v \n", walletAmount)
			}
		}
	}
}

func updateBars(algo *Algo, ex iex.IExchange, trade iex.TradeBin) {
	if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		diff := trade.Timestamp.Sub(time.Unix(database.GetBars()[algo.Index].Timestamp/1000, 0))
		if diff.Minutes() >= 60 {
			database.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, 2)
		}
	} else if algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		database.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, 2)
	} else {
		log.Fatal("This rebalance interval is not supported")
	}
	algo.Index = len(database.GetBars()) - 1
}

func updateState(algo *Algo, ex iex.IExchange, trade iex.TradeBin, setupData func([]*Bar, Algo)) {
	logger.Info("Trade Update:", trade)
	setupData(data.GetBars(), *algo)
	algo.Timestamp = time.Unix(data.GetBars()[algo.Index].Timestamp/1000, 0).UTC()
	algo.Market.Price = *data.GetBars()[algo.Index]
	logger.Info("algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if firstTrade {
		logState(algo)
		firstTrade = false
	}
	if algo.Market.Options {
		algo.TheoEngine.UpdateActiveContracts()
		algo.TheoEngine.ScanOptions(true, true)
	}
}
