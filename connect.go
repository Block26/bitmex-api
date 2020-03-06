package yantra

import (
	"log"
	"math"
	"strings"
	"time"

	"github.com/jinzhu/copier"
	"github.com/tantralabs/exchanges"
	"github.com/tantralabs/logger"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/database"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/tantra"
	"github.com/tantralabs/yantra/utils"
)

var orderStatus iex.OrderStatus
var firstTrade bool
var firstPositionUpdate bool
var commitHash string
var lastTest int64
var lastWalletSync int64
var startTime time.Time
var barData = make(map[string][]*models.Bar)

func RunTest(algo models.Algo, start time.Time, end time.Time, rebalance func(*models.Algo), setupData func(*models.Algo)) {
	exchangeVars := iex.ExchangeConf{
		Exchange:       algo.Account.ExchangeInfo.Exchange,
		ServerUrl:      algo.Account.ExchangeInfo.ExchangeURL,
		AccountID:      "test",
		OutputResponse: false,
	}
	algo.Client = tantra.NewTest(exchangeVars, &algo.Account, start, end, algo.DataLength)
	Connect("", false, algo, rebalance, setupData, true)
}

// Connect is called to connect to an exchange's WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of models.Algo.RebalanceInterval
// This is intentional, look at models.Algo.AutoOrderPlacement to understand this paradigm.
func Connect(settingsFileName string, secret bool, algo models.Algo, rebalance func(*models.Algo), setupData func(*models.Algo), test ...bool) {
	utils.LoadENV(secret)
	startTime = time.Now()
	var isTest bool
	if test != nil {
		isTest = test[0]
		database.Setup()
	} else {
		isTest = false
		database.Setup("remote")
	}
	if algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}
	firstTrade = true
	firstPositionUpdate = true

	commitHash = time.Now().String()

	var err error
	var config models.Secret

	if !isTest {
		config = utils.LoadSecret(settingsFileName, secret)
		logger.Info("Loaded config for", algo.Account.ExchangeInfo.Exchange, "secret", settingsFileName)
		exchangeVars := iex.ExchangeConf{
			Exchange:       algo.Account.ExchangeInfo.Exchange,
			ServerUrl:      algo.Account.ExchangeInfo.ExchangeURL,
			ApiSecret:      config.APISecret,
			ApiKey:         config.APIKey,
			AccountID:      "test",
			OutputResponse: false,
		}
		algo.Client, err = tradeapi.New(exchangeVars)
		if err != nil {
			logger.Error(err)
		}
	}

	orderStatus = algo.Client.GetPotentialOrderStatus()
	var timeSymbol string
	for symbol, marketState := range algo.Account.MarketStates {
		logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", symbol, algo.RebalanceInterval, algo.DataLength+1)
		barData[symbol] = database.GetData(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, algo.DataLength+100)
		marketState.Bar = *barData[symbol][len(barData[symbol])-1]
		marketState.LastPrice = marketState.Bar.Close
		logger.Infof("Initialized bar for %v: %v\n", symbol, marketState.Bar)
		timeSymbol = symbol
	}
	// Set initial timestamp for algo
	if timeSymbol == "" {
		log.Fatal("No bar data.\n")
	}
	algo.Timestamp = time.Unix(barData[timeSymbol][algo.Index].Timestamp/1000, 0).UTC()
	algo.Client.(*tantra.Tantra).SetCurrentTime(algo.Timestamp)

	if algo.Account.ExchangeInfo.Options {
		// Build theo engine
		// Assume the first futures market we find is the underlying market
		var underlyingMarket *models.MarketState
		for symbol, marketState := range algo.Account.MarketStates {
			if marketState.Info.MarketType == models.Future {
				underlyingMarket = marketState
				logger.Infof("Found underlying market: %v\n", symbol)
				break
			}
		}
		if underlyingMarket == nil {
			log.Fatal("Could not find underlying market for options exchange %v\n", algo.Account.ExchangeInfo.Exchange)
		}
		theoEngine := te.NewTheoEngine(underlyingMarket, algo.Client, &algo.Timestamp, 60000, 86400000, 0, 0, algo.LogLevel)
		algo.TheoEngine = &theoEngine
		if isTest {
			theoEngine.CurrentTime = &algo.Client.(*tantra.Tantra).CurrentTime
			algo.Client.(*tantra.Tantra).SetTheoEngine(&theoEngine)
			// theoEngine.UpdateActiveContracts()
			// theoEngine.ApplyVolSurface()
		}
		logger.Infof("Built theo engine.\n")
	}

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := algo.Client.GetBalances()
	updateAlgoBalances(&algo, balances)

	positions, _ := algo.Client.GetPositions(algo.Account.BaseAsset.Symbol)
	updatePositions(&algo, positions)

	orders, err := algo.Client.GetOpenOrders(iex.OpenOrderF{Currency: algo.Account.BaseAsset.Symbol})
	if err != nil {
		logger.Errorf("Error getting open orders: %v\n", err)
	} else {
		logger.Infof("Got %v orders.\n", len(orders))

	}

	// SUBSCRIBE TO WEBSOCKETS
	// channels to subscribe to (only futures and spot for now)
	var subscribeInfos []iex.WSSubscribeInfo
	for marketSymbol, marketState := range algo.Account.MarketStates {
		if marketState.Info.MarketType == models.Future || marketState.Info.MarketType == models.Spot {
			symbol := strings.ToLower(marketSymbol)
			//Ordering is important, get wallet and position first then market info
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_WALLET, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_ORDER, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_POSITION, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol})
		}
	}

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan: make(chan []iex.WsPosition, 2),
		TradeBinChan: make(chan []iex.TradeBin, 2),
		WalletChan:   make(chan *iex.WSWallet, 2),
		OrderChan:    make(chan []iex.Order, 2),
	}

	// Start the websocket.
	err = algo.Client.StartWS(&iex.WsConfig{
		Host:      algo.Account.ExchangeInfo.WSStream,
		Streams:   subscribeInfos,
		Channels:  channels,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	if err != nil {
		logger.Error(err)
	}

	// All of these channels send themselves back so that the test can wait for each individual to complete
	for {
		select {
		case positions := <-channels.PositionChan:
			updatePositions(&algo, positions)
			channels.PositionChan <- positions
		case trade := <-channels.TradeBinChan:
			// Update your local bars
			updateBars(&algo, trade[0])
			// now fetch the bars
			bars := database.GetBars()
			algo.OHLCV = utils.GetOHLCV(bars)
			symbol := trade[0].Symbol

			// Did we get enough data to run this? If we didn't then throw fatal error to notify system
			if algo.DataLength < len(bars) {
				updateState(&algo, symbol, bars, setupData)
				rebalance(&algo)
			} else {
				log.Fatalln("I do not have enough data to trade. local data length", len(bars), "data length wanted by algo", algo.DataLength)
			}
			// setupOrders(&algo, trade[0].Close)
			// placeOrdersOnBook(&algo, localOrders)
			marketState, _ := algo.Account.MarketStates[symbol]
			logState(&algo, marketState)
			if !isTest {
				logLiveState(&algo, marketState)
				runTest(&algo, setupData, rebalance)
				checkWalletHistory(&algo, settingsFileName)
			} else {
				if algo.Timestamp == algo.Client.(*tantra.Tantra).GetLastTimestamp().UTC() {
					channels.TradeBinChan = nil
				} else {
					channels.TradeBinChan <- trade
				}
			}
		case newOrders := <-channels.OrderChan:
			// TODO look at the response for a market order, does it send 2 orders filled and placed or just filled
			updateOrders(&algo, newOrders, true)
			// TODO callback to order function
			channels.OrderChan <- newOrders
		case update := <-channels.WalletChan:
			updateAlgoBalances(&algo, update.Balance)
			channels.WalletChan <- update
		}
		if channels.TradeBinChan == nil {
			break
		}
	}
}

func checkWalletHistory(algo *models.Algo, settingsFileName string) {
	timeSinceLastSync := database.GetBars()[algo.Index].Timestamp - lastWalletSync
	if timeSinceLastSync > (60 * 60 * 60) {
		logger.Info("It has been", timeSinceLastSync, "seconds since the last wallet history download, fetching latest deposits and withdrawals.")
		lastWalletSync = database.GetBars()[algo.Index].Timestamp
		walletHistory, err := algo.Client.GetWalletHistory(algo.Account.BaseAsset.Symbol)
		if err != nil {
			logger.Error("There was an error fetching the wallet history", err)
		} else {
			if len(walletHistory) > 0 {
				database.LogWalletHistory(algo, settingsFileName, walletHistory)
			}
		}
	}
}

// Inject orders directly into market state upon update
func updateOrders(algo *models.Algo, orders []iex.Order, isUpdate bool) {
	if isUpdate {
		// Add to existing order state
		for _, newOrder := range orders {
			marketState, ok := algo.Account.MarketStates[newOrder.Symbol]
			if !ok {
				logger.Errorf("New order symbol %v not found in account market states\n", newOrder.Symbol)
				continue
			}
			marketState.Orders[newOrder.OrderID] = &newOrder
		}
	} else {
		// Overwrite all order states
		openOrderMap := make(map[string]map[string]*iex.Order)
		var orderMap map[string]*iex.Order
		var ok bool
		for _, order := range orders {
			orderMap, ok = openOrderMap[order.Symbol]
			if !ok {
				openOrderMap[order.Symbol] = make(map[string]*iex.Order)
				orderMap = openOrderMap[order.Symbol]
			}
			orderMap[order.OrderID] = &order
		}
		for symbol, marketState := range algo.Account.MarketStates {
			orderMap, ok := openOrderMap[symbol]
			if ok {
				marketState.Orders = orderMap
				logger.Infof("Set orders for %v.\n", symbol)
			} else {
				marketState.Orders = make(map[string]*iex.Order)
			}
		}
	}
}

// TODO do we just want to do a tantra test here?
func runTest(algo *models.Algo, setupData func(*models.Algo), rebalance func(*models.Algo)) {
	if lastTest != database.GetBars()[algo.Index].Timestamp {
		lastTest = database.GetBars()[algo.Index].Timestamp
		testAlgo := models.Algo{}
		copier.Copy(&testAlgo, &algo)
		logger.Info(testAlgo.Account.BaseAsset.Quantity)
		// RESET Algo but leave base balance
		for _, marketState := range testAlgo.Account.MarketStates {
			marketState.Position = 0
			marketState.Leverage = 0
			marketState.Weight = 0
		}
		// Override logger level to info so that we don't pollute logs with backtest state changes
		// testAlgo = RunBacktest(database.GetBars(), testAlgo, rebalance, setupData)
		// logLiveState(&testAlgo, true)
		//TODO compare the states
	}
}

func updatePositions(algo *models.Algo, positions []iex.WsPosition) {
	logger.Info("Position Update:", positions)
	if len(positions) > 0 {
		for _, position := range positions {
			if position.Symbol == algo.Account.BaseAsset.Symbol {
				algo.Account.BaseAsset.Quantity = position.CurrentQty
				logger.Infof("Updated base asset %v: %v\n", algo.Account.BaseAsset.Symbol, algo.Account.BaseAsset.Quantity)
			} else {
				marketState, ok := algo.Account.MarketStates[position.Symbol]
				if !ok {
					logger.Errorf("Got position update %v for symbol %v, could not find in account market states.\n", position, position.Symbol)
					continue
				}
				marketState.Position = position.CurrentQty
				if math.Abs(marketState.Position) > 0 && position.AvgCostPrice > 0 {
					marketState.AverageCost = position.AvgCostPrice
				} else if position.CurrentQty == 0 {
					marketState.AverageCost = 0
				}
				logger.Infof("Got position update for %v with quantity %v, average cost %v\n",
					position.Symbol, marketState.Position, marketState.AverageCost)
				if firstPositionUpdate {
					marketState.ShouldHaveQuantity = marketState.Position
				}
				logState(algo, marketState)
			}
		}
	}
	firstPositionUpdate = false
}

func updateAlgoBalances(algo *models.Algo, balances []iex.WSBalance) {
	logger.Info("updateAlgoBalances")

	for _, updatedBalance := range balances {
		balance, ok := algo.Account.Balances[updatedBalance.Asset]
		if ok {
			balance.Quantity = updatedBalance.Balance
		} else {
			// If unknown asset, create a new asset
			newAsset := models.Asset{
				Symbol:   updatedBalance.Asset,
				Quantity: updatedBalance.Balance,
			}
			algo.Account.Balances[updatedBalance.Asset] = &newAsset
			logger.Infof("New balance found: %v\n", algo.Account.Balances[updatedBalance.Asset])
		}
		if updatedBalance.Asset == algo.Account.BaseAsset.Symbol {
			algo.Account.BaseAsset.Quantity = updatedBalance.Balance
			logger.Infof("Updated base asset quantity: %v\n", algo.Account.BaseAsset.Quantity)
		} else if algo.Account.ExchangeInfo.Spot {
			// This could be a spot position update, in which case we should update the respective market state's position
			for symbol, marketState := range algo.Account.MarketStates {
				if marketState.Info.MarketType == models.Spot && marketState.Info.QuoteSymbol == updatedBalance.Asset {
					marketState.Position = updatedBalance.Balance
					logger.Infof("Updated position for spot market %v: %v\n", symbol, marketState.Position)
				}
			}
		}
	}
}

func updateBars(algo *models.Algo, trade iex.TradeBin) {
	if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		diff := trade.Timestamp.Sub(time.Unix(database.GetBars()[algo.Index].Timestamp/1000, 0))
		if diff.Minutes() >= 60 {
			database.UpdateBars(algo.Client, trade.Symbol, algo.RebalanceInterval, 1)
		}
	} else if algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		database.UpdateBars(algo.Client, trade.Symbol, algo.RebalanceInterval, 1)
	} else {
		log.Fatal("This rebalance interval is not supported")
	}
	algo.Index = len(database.GetBars()) - 1
	logger.Info("Time Elapsed", startTime.Sub(time.Now()), "Index", algo.Index)
}

func updateState(algo *models.Algo, symbol string, bars []*models.Bar, setupData func(*models.Algo)) {
	marketState, ok := algo.Account.MarketStates[symbol]
	if !ok {
		logger.Errorf("Cannot update state for %v (could not find market state).\n", symbol)
		return
	}
	setupData(algo)
	algo.Timestamp = time.Unix(bars[algo.Index].Timestamp/1000, 0).UTC()
	marketState.Bar = *bars[algo.Index]
	marketState.LastPrice = marketState.Bar.Close
	// logger.Info("algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if firstTrade {
		logState(algo, marketState)
		firstTrade = false
	}
	if algo.Account.ExchangeInfo.Options {
		algo.TheoEngine.(*te.TheoEngine).UpdateActiveContracts()
		algo.TheoEngine.(*te.TheoEngine).ScanOptions(true, true)
	}
}
