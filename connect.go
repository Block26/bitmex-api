package yantra

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/data"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/logger"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/options"
	"github.com/tantralabs/yantra/utils"

	"github.com/jinzhu/copier"
)

var orderStatus iex.OrderStatus
var firstTrade bool
var firstPositionUpdate bool
var commitHash string
var lastTest int64

// Connect is called to connect to an exchanges WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of Algo.RebalanceInterval
//
// This is intentional, look at Algo.AutoOrderPlacement to understand this paradigm.
func Connect(settingsFile string, secret bool, algo Algo, rebalance func(Algo) Algo, setupData func([]*models.Bar, Algo)) {
	data.Setup("remote")
	if algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}
	firstTrade = true
	firstPositionUpdate = true
	config := utils.LoadConfiguration(settingsFile, secret)
	logger.Infof("Loaded config for %v \n", algo.Market.Exchange)
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
	localBars := data.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, algo.DataLength+100)
	// Set initial timestamp for algo
	algo.Timestamp = time.Unix(data.GetBars()[algo.Index].Timestamp/1000, 0).UTC().String()
	logger.Infof("Got local bars: %v\n", len(localBars))
	// logger.Infof(len(localBars), "downloaded")

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := ex.GetBalances()
	algo.updateAlgoBalances(balances)

	//Get Option contracts before updating positions
	algo.getOptionContracts(ex)

	positions, _ := ex.GetPositions(algo.Market.BaseAsset.Symbol)
	algo.updatePositions(positions)

	var localOrders []iex.Order
	orders, _ := ex.GetOpenOrders(iex.OpenOrderF{Currency: algo.Market.BaseAsset.Symbol})
	localOrders = updateLocalOrders(localOrders, orders)
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
			algo.updatePositions(positions)
		case trade := <-channels.TradeBinChan:
			algo.updateBars(ex, trade[0])
			algo.updateState(ex, trade[0], setupData)
			algo = rebalance(algo)
			algo.setupOrders()
			algo.placeOrdersOnBook(ex, localOrders)
			algo.logState()
			algo.runTest(setupData, rebalance)
		case newOrders := <-channels.OrderChan:
			localOrders = updateLocalOrders(localOrders, newOrders)
		case update := <-channels.WalletChan:
			algo.updateAlgoBalances(update.Balance)
		}
	}
}

func (algo *Algo) runTest(setupData func([]*models.Bar, Algo), rebalance func(Algo) Algo) {
	if lastTest != data.GetBars()[algo.Index].Timestamp {
		lastTest = data.GetBars()[algo.Index].Timestamp
		testAlgo := Algo{}
		copier.Copy(&testAlgo, &algo)
		logger.Info(testAlgo.Market.BaseAsset.Quantity)
		// RESET Algo but leave base balance
		testAlgo.Market.QuoteAsset.Quantity = 0
		testAlgo.Market.Leverage = 0
		testAlgo.Market.Weight = 0
		// Override logger level to info so that we don't pollute logs with backtest state changes
		testAlgo = RunBacktest(data.GetBars(), testAlgo, rebalance, setupData)
		testAlgo.logLiveState(true)
		//TODO compare the states
	}

}

func (algo *Algo) updatePositions(positions []iex.WsPosition) {
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
			algo.logState()
			algo.shouldHaveQuantity = algo.Market.QuoteAsset.Quantity
			firstPositionUpdate = false
		}
	}
	algo.logState()
}

func (algo *Algo) updateAlgoBalances(balances []iex.WSBalance) {
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

func (algo *Algo) updateBars(ex iex.IExchange, trade iex.TradeBin) {
	if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		diff := trade.Timestamp.Sub(time.Unix(data.GetBars()[algo.Index].Timestamp/1000, 0))
		if diff.Minutes() >= 60 {
			data.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, 2)
		}
	} else if algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		data.UpdateBars(ex, algo.Market.Symbol, algo.RebalanceInterval, 2)
	} else {
		log.Fatal("This rebalance interval is not supported")
	}
	algo.Index = len(data.GetBars()) - 1
}

func (algo *Algo) updateState(ex iex.IExchange, trade iex.TradeBin, setupData func([]*models.Bar, Algo)) {
	logger.Info("Trade Update:", trade)
	algo.Market.Price = *data.GetBars()[algo.Index]
	setupData(data.GetBars(), *algo)
	algo.Timestamp = time.Unix(data.GetBars()[algo.Index].Timestamp/1000, 0).UTC().String()
	logger.Info("algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if firstTrade {
		algo.logState()
		firstTrade = false
	}
	// Update active option contracts from API
	algo.getOptionContracts(ex)
}

func (algo *Algo) getOptionContracts(ex iex.IExchange) {
	if algo.Market.Exchange == "deribit" && algo.Market.Options {
		// TODO only call this every few hours or once per day.
		markets, err := ex.GetMarkets(algo.Market.BaseAsset.Symbol, true, "option")
		if err == nil {
			// logger.Infof("Got markets from API: %v\n", markets)
			for _, market := range markets {
				containsSymbol := false
				for _, option := range algo.Market.OptionContracts {
					if option.Symbol == market.Symbol {
						containsSymbol = true
					}
				}
				if !containsSymbol {
					// expiry := market.Expiry * 1000
					expiry := market.Expiry
					optionTheo := models.NewOptionTheo(market.OptionType, algo.Market.Price.Close, market.Strike, utils.ToIntTimestamp(algo.Timestamp), expiry, 0, -1, -1, algo.Market.DenominatedInUnderlying)
					optionContract := models.OptionContract{
						Symbol:         market.Symbol,
						Strike:         market.Strike,
						Expiry:         expiry,
						OptionType:     market.OptionType,
						AverageCost:    0,
						Profit:         0,
						Position:       0,
						OptionTheo:     *optionTheo,
						Status:         "open",
						MidMarketPrice: market.MidMarketPrice,
					}
					// fmt.Printf("Set mid market price for %v: %v\n", market.Symbol, market.MidMarketPrice)
					algo.Market.OptionContracts = append(algo.Market.OptionContracts, optionContract)
				}
			}
		} else {
			logger.Errorf("Error getting markets: %v\n", err)
		}
		var optionContracts []*models.OptionContract
		for i := 0; i < len(algo.Market.OptionContracts); i++ {
			optionContracts = append(optionContracts, &algo.Market.OptionContracts[i])
		}
		options.SetMidMarketVols(optionContracts)
		options.PropagateVolatility(optionContracts, options.DefaultVolatility)
		logger.Infof("Propagated volatility for %v options.\n", len(algo.Market.OptionContracts))
		for _, option := range algo.Market.OptionContracts {
			if option.OptionTheo.Volatility < 0 {
				logger.Debugf("Found option with negative volatitlity after propagation: %v\n", option.Symbol)
			}
		}
	}
}
