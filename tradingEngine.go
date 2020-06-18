// The yantra package contains all base layer components of the Tantra Labs algorithmic trading platform.
// The primary compenent is a trading engine used for interfacing with exchanges and managing requests in both
// backtesting and live environments.
package yantra

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/copier"
	"github.com/tantralabs/logger"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/database"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/tantra"
	"github.com/tantralabs/yantra/utils"

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
)

var currentRunUUID time.Time
var barData map[string][]*models.Bar
var index int = 0
var lastTimestamp map[string]int

const additionalLiveData int = 3000
const checkWalletHistoryInterval int = 60
const liveTestInterval int = 15

// The trading engine is responsible for managing communication between algos and other modules and the exchange.
type TradingEngine struct {
	Algo                 *models.Algo
	reuseData            bool
	firstTrade           bool
	firstPositionUpdate  bool
	isTest               bool
	commitHash           string
	lastTest             int64
	lastWalletSync       int64
	startTime            time.Time
	endTime              time.Time
	theoEngine           *te.TheoEngine
	lastContractUpdate   int
	contractUpdatePeriod int
}

// Construct a new trading engine given an algo and other configurations.
func NewTradingEngine(algo *models.Algo, contractUpdatePeriod int, reuseData ...bool) TradingEngine {
	currentRunUUID = time.Now()
	//TODO: should theo engine and other vars be initialized here?
	if reuseData == nil {
		reuseData = make([]bool, 1)
		reuseData[0] = false
	}

	return TradingEngine{
		Algo:                 algo,
		firstTrade:           true,
		firstPositionUpdate:  true,
		commitHash:           time.Now().String(),
		lastTest:             0,
		lastWalletSync:       0,
		startTime:            time.Now(),
		reuseData:            reuseData[0],
		theoEngine:           nil,
		lastContractUpdate:   0,
		contractUpdatePeriod: contractUpdatePeriod,
	}
}

// Run a backtest given a start and end time.
// Provide a rebalance function to be called at every data interval and performs trading logic.
// Optionally, provide a setup data to be called before rebalance to precompute relevant data and metrics for the algo.
// This is the trading engine's entry point for a new backtest.
func (t *TradingEngine) RunTest(start time.Time, end time.Time, rebalance func(*models.Algo), setupData func(*models.Algo), live ...bool) {
	t.SetupTest(start, end, rebalance, setupData, live...)
	t.Connect("", false, rebalance, setupData, true)
}

func (t *TradingEngine) SetupTest(start time.Time, end time.Time, rebalance func(*models.Algo), setupData func(*models.Algo), live ...bool) {
	t.isTest = true
	isLive := false
	if live != nil {
		isLive = true
	}

	logger.SetLogLevel(t.Algo.BacktestLogLevel)
	exchangeVars := iex.ExchangeConf{
		Exchange:       t.Algo.Account.ExchangeInfo.Exchange,
		ServerUrl:      t.Algo.Account.ExchangeInfo.ExchangeURL,
		AccountID:      t.Algo.Name,
		OutputResponse: false,
	}
	mockExchange := tantra.NewTest(exchangeVars, &t.Algo.Account, start, end, t.Algo.DataLength, t.Algo.LogBacktest)
	// If we are live we already have all the data we need so there is no need to fetch it again
	if !isLive {
		barData = t.LoadBarData(t.Algo, start, end)
	}
	for symbol, data := range barData {
		logger.Infof("Loaded %v instances of bar data for %v with start %v and end %v.\n", len(data), symbol, start, end)
	}

	t.SetAlgoCandleData(barData)
	mockExchange.SetCandleData(barData)
	mockExchange.SetCurrentTime(start)
	t.Algo.Client = mockExchange
	t.Algo.Timestamp = start
	t.endTime = end
	setupData(t.Algo)
}

// Set the candle data for the trading engine and format it according to the algo's configuration.
func (t *TradingEngine) SetAlgoCandleData(candleData map[string][]*models.Bar) {
	for symbol, data := range candleData {
		marketState, ok := t.Algo.Account.MarketStates[symbol]
		if !ok {
			logger.Errorf("Cannot set bar data for market state %v.\n", symbol)
		}
		if t.isTest {
			d := models.SetupDataModel(data, t.Algo.DataLength, t.isTest)
			marketState.OHLCV = &d
		} else {
			d := models.SetupDataModel(data, len(data), t.isTest)
			marketState.OHLCV = &d
			b := marketState.OHLCV.GetBarData()
			marketState.Bar = *b[len(b)-1]
		}
	}
}

// Given a new instance of candle data, inject it into the relevant market state for access by the algo's trading logic.
func (t *TradingEngine) InsertNewCandle(candle iex.TradeBin) {
	marketState, ok := t.Algo.Account.MarketStates[candle.Symbol]
	if !ok {
		logger.Errorf("Cannot insert new candle for symbol %v (candle=%v)\n", candle.Symbol, candle)
		return
	}
	if candle.Timestamp.After(t.Algo.Timestamp) {
		t.Algo.Timestamp = candle.Timestamp
	}
	// instead of inserting a new bar in the data all the time in a test
	// just increment the index
	if t.isTest {
		marketState.OHLCV.IncrementIndex()
	} else {
		marketState.OHLCV.AddDataFromTradeBin(candle)
		b := marketState.OHLCV.GetBarData()
		marketState.Bar = *b[len(b)-1]
	}
}

// Given an algo and a start and end time, load relevant candle data from the database.
// The data is returned as a map of symbol to pointers of Bar structs.
func (t *TradingEngine) LoadBarData(algo *models.Algo, start time.Time, end time.Time) map[string][]*models.Bar {
	if barData == nil || !t.reuseData {
		barData = make(map[string][]*models.Bar)
		for symbol, marketState := range algo.Account.MarketStates {
			logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", symbol, algo.RebalanceInterval, algo.DataLength+1)
			// TODO handle extra bars to account for dataLength here
			// barData[symbol] = database.GetData(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, algo.DataLength+100)
			barData[symbol] = database.GetCandlesByTimeWithBuffer(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, start, end, algo.DataLength)
			marketState.Bar = *barData[symbol][len(barData[symbol])-1]
			marketState.LastPrice = marketState.Bar.Close
			logger.Infof("Initialized bar for %v: %v\n", symbol, marketState.Bar)
		}
		return barData
	}
	return barData
}

// Connect is called to connect to an exchange's WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of models.Algo.RebalanceInterval
// This is intentional, look at models.Algo.AutoOrderPlacement to understand this paradigm.
func (t *TradingEngine) Connect(settingsFileName string, secret bool, rebalance func(*models.Algo), setupData func(*models.Algo), test ...bool) {
	startTime := time.Now()
	utils.LoadENV(secret)
	if test != nil {
		t.isTest = test[0]
		database.Setup()
	} else {
		t.isTest = false
		database.Setup("remote")
		logger.SetLogLevel(t.Algo.LogLevel)
	}
	if t.Algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}

	var err error
	var config models.Secret
	history := make([]models.History, 0)
	lastTimestamp := make(map[string]int, 0)

	if !t.isTest {
		config = utils.LoadSecret(settingsFileName, secret)
		logger.Info("Loaded config for", t.Algo.Account.ExchangeInfo.Exchange, "secret", settingsFileName)
		exchangeVars := iex.ExchangeConf{
			Exchange:       t.Algo.Account.ExchangeInfo.Exchange,
			ServerUrl:      t.Algo.Account.ExchangeInfo.ExchangeURL,
			ApiSecret:      config.APISecret,
			ApiKey:         config.APIKey,
			AccountID:      "test",
			OutputResponse: false,
		}
		t.Algo.Client, err = tradeapi.New(exchangeVars)
		//  Fetch prelim data from db to run live
		barData = make(map[string][]*models.Bar)
		for symbol, ms := range t.Algo.Account.MarketStates {
			barData[symbol] = database.GetLatestMinuteData(t.Algo.Client, symbol, ms.Info.Exchange, t.Algo.DataLength+additionalLiveData)
		}
		t.SetAlgoCandleData(barData)
		if err != nil {
			logger.Error(err)
		}
	}

	//TODO do we need this order status?
	// t.orderStatus = algo.Client.GetPotentialOrderStatus()
	if t.Algo.Account.ExchangeInfo.Options {
		// Build theo engine
		// Assume the first futures market we find is the underlying market
		var underlyingMarket *models.MarketState
		for symbol, marketState := range t.Algo.Account.MarketStates {
			if marketState.Info.MarketType == models.Future {
				underlyingMarket = marketState
				logger.Infof("Found underlying market: %v\n", symbol)
				break
			}
		}
		if underlyingMarket == nil {
			log.Fatal("Could not find underlying market for options exchange\n", t.Algo.Account.ExchangeInfo.Exchange)
		}
		t.Algo.Account.MarketStates[underlyingMarket.Symbol] = underlyingMarket
		logger.Infof("Initialized underlying market: %v\n", underlyingMarket.Symbol)
		theoEngine := te.NewTheoEngine(underlyingMarket, &t.Algo.Timestamp, 60000, 86400000, 0, 0, t.Algo.LogLevel)
		t.Algo.TheoEngine = &theoEngine
		t.theoEngine = &theoEngine
		logger.Infof("Built new theo engine.\n")
		if t.isTest {
			theoEngine.CurrentTime = &t.Algo.Client.(*tantra.Tantra).CurrentTime
			t.Algo.Client.(*tantra.Tantra).SetTheoEngine(&theoEngine)
			// theoEngine.UpdateActiveContracts()
			// theoEngine.ApplyVolSurface()
		}
		logger.Infof("Built theo engine.\n")
	} else {
		logger.Infof("Not building theo engine (no options)\n")
	}

	// SETUP Algo WITH RESTFUL CALLS
	balances, _ := t.Algo.Client.GetBalances()
	t.updateAlgoBalances(t.Algo, balances)

	positions, _ := t.Algo.Client.GetPositions(t.Algo.Account.BaseAsset.Symbol)
	t.updatePositions(t.Algo, positions)

	orders, err := t.Algo.Client.GetOpenOrders(iex.OpenOrderF{Currency: t.Algo.Account.BaseAsset.Symbol})
	if err != nil {
		logger.Errorf("Error getting open orders: %v\n", err)
	} else {
		logger.Infof("Got %v orders.\n", len(orders))

	}

	// SUBSCRIBE TO WEBSOCKETS
	// channels to subscribe to (only futures and spot for now)
	var subscribeInfos []iex.WSSubscribeInfo
	for symbol, marketState := range t.Algo.Account.MarketStates {
		if marketState.Info.MarketType == models.Future || marketState.Info.MarketType == models.Spot {
			//Ordering is important, get wallet and position first then market info
			logger.Infof("Subscribing to %v channels.\n", symbol)
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_WALLET})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_ORDER, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_POSITION, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol, Market: iex.WSMarketType{Contract: iex.WS_SWAP}})
		}
	}

	logger.Infof("Subscribed to %v channels.\n", len(subscribeInfos))

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan:         make(chan []iex.WsPosition, 1),
		PositionChanComplete: make(chan error, 1),
		TradeBinChan:         make(chan []iex.TradeBin, 1),
		TradeBinChanComplete: make(chan error, 1),
		WalletChan:           make(chan []iex.Balance, 1),
		WalletChanComplete:   make(chan error, 1),
		OrderChan:            make(chan []iex.Order, 1),
		OrderChanComplete:    make(chan error, 1),
	}

	// Start the websocket.

	ctx := context.TODO()
	wg := sync.WaitGroup{}
	err = t.Algo.Client.StartWS(&iex.WsConfig{
		Host:      t.Algo.Account.ExchangeInfo.WSStream,
		Streams:   subscribeInfos,
		Channels:  channels,
		Ctx:       ctx,
		Wg:        &wg,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	if err != nil {
		msg := fmt.Sprintf("Error starting websockets: %v\n", err)
		log.Fatal(msg)
	}

	// All of these channels send themselves back so that the test can wait for each individual to complete
	for {
		select {
		case positions := <-channels.PositionChan:
			t.updatePositions(t.Algo, positions)
			if t.isTest {
				channels.PositionChanComplete <- nil
			}
		case trades := <-channels.TradeBinChan:
			// Make sure the trades are coming from the exchange in the right order.
			ts, ok := lastTimestamp["trades"]
			currentTimestamp := int(trades[0].Timestamp.Unix())
			if ok {
				if ts < currentTimestamp {
					lastTimestamp["trades"] = currentTimestamp
				} else {
					log.Fatalln("ERROR recieved an trade out of order...", ts, currentTimestamp)
					return
				}
			} else {
				lastTimestamp["trades"] = currentTimestamp
			}

			startTimestamp := time.Now().Unix()
			logger.Debugf("Recieved %v new trade updates: %v\n", len(trades), trades)
			// Update active contracts if we are trading options
			// if t.theoEngine != nil {
			// 	t.UpdateActiveContracts()
			// 	t.UpdateMidMarketPrices()
			// 	t.theoEngine.ScanOptions(false, true)
			// } else {
			// 	logger.Debugf("Cannot update active contracts, theo engine is nil\n")
			// }
			// Update your local bars
			for _, trade := range trades {
				t.InsertNewCandle(trade)
				marketState, _ := t.Algo.Account.MarketStates[trade.Symbol]
				// Did we get enough data to run this? If we didn't then throw fatal error to notify system
				if t.Algo.DataLength < len(marketState.OHLCV.GetMinuteData().Timestamp) {
					t.updateState(t.Algo, trade.Symbol, setupData)
					rebalance(t.Algo)
				} //else {
				// log.Println("Not enough trade data. (local data length", len(marketState.OHLCV.GetMinuteData().Timestamp), "data length wanted by Algo", t.Algo.DataLength, ")")
				// }
			}
			for _, marketState := range t.Algo.Account.MarketStates {
				state := logState(t.Algo, marketState)
				history = append(history, state)
				if !t.isTest {
					t.logLiveState()
				}
			}
			if !t.isTest {
				// t.runTest(t.Algo, setupData, rebalance)
				t.checkWalletHistory(t.Algo, settingsFileName)
			}
			t.aggregateAccountProfit()
			logger.Debugf("[Trading Engine] trade processing took %v s\n", (time.Now().Unix() - startTimestamp))
			logger.Debug("===========================================")
			logger.Debugf("Fetching positions now as a stop gap")
			positions, _ := t.Algo.Client.GetPositions(t.Algo.Account.BaseAsset.Symbol)
			t.updatePositions(t.Algo, positions)
			if t.isTest {
				channels.TradeBinChanComplete <- nil
			} else {
				index++
			}
			// log.Println("t.isTest", t.isTest, "t.endTime", t.endTime, "t.Algo.Timestamp", t.Algo.Timestamp, !t.Algo.Timestamp.Before(t.endTime))
			if !t.Algo.Timestamp.Before(t.endTime) && t.isTest {
				logger.Infof("Algo timestamp %v past end time %v, killing trading engine.\n", t.Algo.Timestamp, t.endTime)
				logStats(t.Algo, history, startTime)
				logBacktest(t.Algo)
				return
			}
		case newOrders := <-channels.OrderChan:
			// Make sure the orders are coming from the exchange in the right order.
			ts, ok := lastTimestamp["orders"]
			currentTimestamp := int(newOrders[0].TransactTime.Unix())
			if ok {
				if ts <= currentTimestamp {
					lastTimestamp["orders"] = currentTimestamp
				} else {
					fmt.Println("ERROR recieved an order out of order...", ts, currentTimestamp)
					return
				}
			} else {
				lastTimestamp["orders"] = currentTimestamp
			}
			// startTimestamp := time.Now().UnixNano()
			// logger.Infof("Recieved %v new order updates\n", len(newOrders))
			// TODO look at the response for a market order, does it send 2 orders filled and placed or just filled
			t.updateOrders(t.Algo, newOrders, true)
			// TODO callback to order function
			// logger.Infof("Order processing took %v ns\n", time.Now().UnixNano()-startTimestamp)
			if t.isTest {
				channels.OrderChanComplete <- nil
			}
		case update := <-channels.WalletChan:
			t.updateAlgoBalances(t.Algo, update)
			if t.isTest {
				channels.WalletChanComplete <- nil
			}
		}
		if channels.TradeBinChan == nil {
			logger.Errorf("Trade bin channel is nil, breaking...\n")
			break
		}
	}
	logger.Infof("Reached end of connect.\n")
}

// Get the wallet history from the exchange and log it to the db.
// Don't log this data if we have already logged within walletHistoryPeriod seconds.
func (t *TradingEngine) checkWalletHistory(algo *models.Algo, settingsFileName string) {
	// Check for new wallet entry every 60m
	if index%checkWalletHistoryInterval == 0 {
		// logger.Info("It has been", timeSinceLastSync, "seconds since the last wallet history download, fetching latest deposits and withdrawals.")
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

// Inject orders directly into market state upon update.
func (t *TradingEngine) updateOrders(algo *models.Algo, orders []iex.Order, isUpdate bool) {
	// logger.Infof("Processing %v order updates.\n", len(orders))
	if isUpdate {
		// Add to existing order state
		for _, newOrder := range orders {
			if newOrder.OrdStatus != t.Algo.Client.GetPotentialOrderStatus().Cancelled {
				logger.Debugf("Processing order update: %v\n", newOrder.Symbol)
				marketState, ok := algo.Account.MarketStates[newOrder.Symbol]
				if !ok {
					logger.Errorf("New order symbol %v not found in account market states\n", newOrder.Symbol)
					continue
				}
				marketState.Orders.Store(newOrder.OrderID, newOrder)
			}
		}
	} else {
		// Overwrite all order states
		openOrderMap := make(map[string]map[string]iex.Order)
		var orderMap map[string]iex.Order
		var ok bool
		for _, order := range orders {
			orderMap, ok = openOrderMap[order.Symbol]
			if !ok {
				openOrderMap[order.Symbol] = make(map[string]iex.Order)
				orderMap = openOrderMap[order.Symbol]
			}
			orderMap[order.OrderID] = order
		}
		for symbol, marketState := range algo.Account.MarketStates {
			orderMap, ok := openOrderMap[symbol]
			if ok {
				for id, order := range orderMap {
					marketState.Orders.Store(id, order)
				}
				logger.Infof("Set orders for %v.\n", symbol)
			}
		}
	}
}

// Run a new backtest. This private function is meant to be called while the trading engine is running live, as a means
// of making sure that the current state is similar to the expected state (a safety mechanism).
func (t *TradingEngine) runTest(algo *models.Algo, setupData func(*models.Algo), rebalance func(*models.Algo)) {
	// Run a test every 15m
	if index%liveTestInterval == 0 {
		testEngine := TradingEngine{}
		copier.Copy(&testEngine, &t)
		// RESET Algo but leave base balance
		for _, marketState := range testEngine.Algo.Account.MarketStates {
			marketState.Position = 0
			marketState.Leverage = 0
			marketState.Weight = 0
		}
		now := time.Now().Local().UTC()
		var start time.Time

		start = now.Add(time.Duration(-(t.Algo.DataLength + additionalLiveData)) * time.Minute)
		end := t.Algo.Timestamp.Add(time.Duration(-1) * time.Minute)
		testEngine.RunTest(start, end, rebalance, setupData, true)
		testEngine.logLiveState(true)
		//TODO compare the states
	}
}

// Given a set of websocket position updates, update all relevant market states.
func (t *TradingEngine) updatePositions(algo *models.Algo, positions []iex.WsPosition) {
	logger.Debug("Position Update:", positions)
	if len(positions) > 0 {
		for _, position := range positions {
			if position.Symbol == algo.Account.BaseAsset.Symbol {
				algo.Account.BaseAsset.Quantity = position.CurrentQty
				logger.Debugf("Updated base asset %v: %v\n", algo.Account.BaseAsset.Symbol, algo.Account.BaseAsset.Quantity)
			} else {
				t.updateStatePosition(algo, position)
			}
		}
	}
	t.firstPositionUpdate = false
}

// Update a single market's position given a websocket position update.
func (t *TradingEngine) updateStatePosition(algo *models.Algo, position iex.WsPosition) {
	marketState, ok := algo.Account.MarketStates[position.Symbol]
	if !ok {
		logger.Errorf("Got position update %v for symbol %v, could not find in account market states.\n", position, position.Symbol)
	}
	marketState.Position = position.CurrentQty
	if math.Abs(marketState.Position) > 0 && position.AvgCostPrice > 0 {
		marketState.AverageCost = position.AvgCostPrice

	} else if position.CurrentQty == 0 {
		marketState.AverageCost = 0
	}
	marketState.UnrealizedProfit = getPositionAbsProfit(algo, marketState)
	logger.Debugf("Got position update for %v with quantity %v, average cost %v\n",
		position.Symbol, marketState.Position, marketState.AverageCost)
	if t.firstPositionUpdate {
		marketState.ShouldHaveQuantity = marketState.Position
	}

	var balance float64
	if marketState.Info.MarketType == models.Future {
		if algo.Account.ExchangeInfo.DenominatedInQuote {
			position := (math.Abs(marketState.Position) * marketState.Bar.Close)
			marketState.Leverage = (position / (marketState.Balance + position))
			balance = (math.Abs(marketState.Position) * marketState.Bar.Close) + algo.Account.BaseAsset.Quantity
			// marketState.Leverage = balance / marketState.Balance
			log.Println("BTC Position", marketState.Position, "USDT Balance", marketState.Balance, "UBalance", balance, "Leverage", marketState.Leverage, "Price", marketState.Bar.Close)
		} else {
			balance = algo.Account.BaseAsset.Quantity
			marketState.Leverage = math.Abs(marketState.Position) / (marketState.Bar.Close * balance)
		}

	} else {
		if marketState.AverageCost == 0 {
			marketState.AverageCost = marketState.Bar.Close
		}
		balance = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		marketState.Leverage = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) / balance
	}
	marketState.Profit = marketState.UnrealizedProfit //+ marketState.RealizedProfit

}

// Iterate through all visible market states and calculate unrealized, realized, and total profits across all markets.
func (t *TradingEngine) aggregateAccountProfit() {
	totalUnrealizedProfit := 0.
	totalRealizedProfit := 0.
	for _, marketState := range t.Algo.Account.MarketStates {
		// TODO Should this be calculated or grabbed from the exchange?
		totalUnrealizedProfit += marketState.UnrealizedProfit
		totalRealizedProfit += marketState.RealizedProfit
	}
	t.Algo.Account.UnrealizedProfit = totalUnrealizedProfit
	t.Algo.Account.RealizedProfit = totalRealizedProfit
	t.Algo.Account.Profit = totalUnrealizedProfit + totalRealizedProfit
	logger.Debugf("Aggregated account unrealized profit: %v, realized profit: %v, total profit: %v, balance: %v\n",
		t.Algo.Account.UnrealizedProfit, t.Algo.Account.RealizedProfit, t.Algo.Account.Profit, t.Algo.Account.BaseAsset.Quantity)
}

// Update all balances contained by the trading engine given a slice of websocket balance updates from the exchange.
func (t *TradingEngine) updateAlgoBalances(algo *models.Algo, balances []iex.Balance) {
	// fmt.Println("updateAlgoBalances")
	for _, updatedBalance := range balances {
		balance, ok := algo.Account.Balances[updatedBalance.Currency]
		if ok {
			balance.Quantity = updatedBalance.Balance
		} else {
			// If unknown asset, create a new asset
			newAsset := models.Asset{
				Symbol:   updatedBalance.Currency,
				Quantity: updatedBalance.Balance,
			}
			algo.Account.Balances[updatedBalance.Currency] = &newAsset
			logger.Debugf("New balance found: %v\n", algo.Account.Balances[updatedBalance.Currency])
		}
		if updatedBalance.Currency == algo.Account.BaseAsset.Symbol {
			algo.Account.BaseAsset.Quantity = updatedBalance.Balance
			// logger.Debugf("Updated base asset quantity: %v\n", algo.Account.BaseAsset.Quantity)
		} else if algo.Account.ExchangeInfo.Spot {
			// This could be a spot position update, in which case we should update the respective market state's position
			for symbol, marketState := range algo.Account.MarketStates {
				if marketState.Info.MarketType == models.Spot && marketState.Info.QuoteSymbol == updatedBalance.Currency {
					marketState.Position = updatedBalance.Balance
					logger.Debugf("Updated position for spot market %v: %v\n", symbol, marketState.Position)
				}
			}
		}
	}
}

// Setup data for a given market, and log the state of the algo to the db if necessary.
func (t *TradingEngine) updateState(algo *models.Algo, symbol string, setupData func(*models.Algo)) {
	marketState, ok := algo.Account.MarketStates[symbol]
	if !ok {
		logger.Errorf("Cannot update state for %v (could not find market state).\n", symbol)
		return
	}
	if !t.isTest {
		setupData(algo)
	}
	// logger.Info("Algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if t.firstTrade {
		logState(algo, marketState)
		t.firstTrade = false
	}
}

// Get new contracts from the exchange and remove all expired ones. Don't do anything
// if enough time has not passed (contractUpdatePeriod).
// This method is really only useful for options, or other deriviatives with expirations.
func (t *TradingEngine) UpdateActiveContracts() {
	logger.Infof("Updating active contracts at %v\n", t.Algo.Timestamp)
	updateTime := t.lastContractUpdate + t.contractUpdatePeriod
	currentTimestamp := utils.TimeToTimestamp(t.Algo.Timestamp)
	if updateTime > currentTimestamp {
		logger.Infof("Skipping contract update. (next update at %v, current time %v)\n", updateTime, currentTimestamp)
		return
	}
	activeOptions := t.GetActiveOptions()
	logger.Infof("Found %v new active options.\n", len(activeOptions))
	var ok bool
	for symbol, marketState := range activeOptions {
		// TODO is this check necessary? may already happen in GetActiveContracts()
		marketStateCopy := marketState
		_, ok = t.theoEngine.Options[symbol]
		if !ok {
			t.theoEngine.Options[symbol] = &marketStateCopy
			// logger.Debugf("New option found for symbol %v: %p\n", symbol, t.theoEngine.Options[symbol])
		}
		_, ok = t.Algo.Account.MarketStates[symbol]
		if !ok {
			t.Algo.Account.MarketStates[symbol] = &marketStateCopy
		}
	}
	t.RemoveExpiredOptions()
	t.theoEngine.UpdateOptionIndexes()
	t.lastContractUpdate = currentTimestamp
}

// Get a map of all currently open option contracts on the exchange.
func (t *TradingEngine) GetActiveOptions() map[string]models.MarketState {
	logger.Infof("Generating active contracts at %v\n", t.Algo.Timestamp)
	liveContracts := make(map[string]models.MarketState)
	var marketInfo models.MarketInfo
	var marketState models.MarketState
	var optionType models.OptionType
	if t.Algo.Account.ExchangeInfo.Exchange == "deribit" {
		markets, err := t.Algo.Client.GetMarkets(t.Algo.Account.BaseAsset.Symbol, true, "option")
		logger.Infof("Got %v markets.\n", len(markets))
		if err == nil {
			for _, market := range markets {
				_, ok := t.theoEngine.Options[market.Symbol]
				if !ok {
					optionTheo := models.NewOptionTheo(
						market.Strike,
						t.theoEngine.UnderlyingPrice,
						market.Expiry,
						market.OptionType,
						t.theoEngine.Market.Info.DenominatedInUnderlying,
						t.theoEngine.CurrentTime,
					)
					if market.OptionType == "call" {
						optionType = models.Call
					} else if market.OptionType == "put" {
						optionType = models.Put
					} else {
						logger.Errorf("Unknown option type: %v\n", market.OptionType)
						continue
					}
					marketInfo = models.NewMarketInfo(market.Symbol, t.theoEngine.ExchangeInfo)
					marketInfo.BaseSymbol = t.theoEngine.Market.Info.BaseSymbol
					marketInfo.QuoteSymbol = t.theoEngine.Market.Info.QuoteSymbol
					marketInfo.Expiry = market.Expiry
					marketInfo.Strike = market.Strike
					marketInfo.OptionType = optionType
					marketInfo.MarketType = models.Option
					marketState = models.NewMarketStateFromInfo(marketInfo, t.Algo.Account.BaseAsset.Quantity)
					marketState.MidMarketPrice = market.MidMarketPrice
					marketState.OptionTheo = &optionTheo
					marketState.Info.MinimumOrderSize = t.Algo.Account.ExchangeInfo.MinimumOrderSize
					liveContracts[marketState.Symbol] = marketState
					logger.Debugf("Set mid market price for %v: %v\n", market.Symbol, market.MidMarketPrice)
				}
			}
		} else {
			logger.Errorf("Error getting markets: %v\n", err)
		}
	} else {
		logger.Errorf("GetOptionsContracts() not implemented for exchange %v\n", t.Algo.Account.ExchangeInfo.Exchange)
	}
	logger.Infof("Found %v live contracts.\n", len(liveContracts))
	return liveContracts
}

// Update the mid market prices of all tradable contracts. Mostly only useful for options.
func (t *TradingEngine) UpdateMidMarketPrices() {
	if t.Algo.Account.ExchangeInfo.Options {
		logger.Debugf("Updating mid markets at %v with currency %v\n", t.Algo.Timestamp, t.Algo.Account.BaseAsset.Symbol)
		// marketPrices, err := t.Algo.Client.GetMarketPricesByCurrency(t.Algo.Account.BaseAsset.Symbol)
		// if err != nil {
		// 	logger.Errorf("Error getting market prices for %v: %v\n", t.Algo.Account.BaseAsset.Symbol, err)
		// 	return
		// }
		// for symbol, price := range marketPrices {
		// 	option, ok := t.theoEngine.Options[symbol]
		// 	if ok {
		// 		option.MidMarketPrice = price
		// 	}
		// }
	} else {
		logger.Infof("Exchange does not support options, no need to update mid market prices.\n")
	}
}

// Delete all expired options without profit values to conserve time and space resources.
func (t *TradingEngine) RemoveExpiredOptions() {
	numOptions := len(t.theoEngine.Options)
	for symbol, option := range t.theoEngine.Options {
		if option.Info.Expiry <= t.theoEngine.GetCurrentTimestamp() {
			option.Status = models.Expired
		}
		if option.Status == models.Expired && option.Profit == 0. {
			delete(t.theoEngine.Options, symbol)
			//TODO delete from algo.Accounts here or keep for history?
			logger.Infof("Removed expired option: %v\n", symbol)
		}
	}
	logger.Infof("Removed %v expired option contracts; %v contracts remain.\n", numOptions-len(t.theoEngine.Options), len(t.theoEngine.Options))
	t.theoEngine.UpdateOptionIndexes()
}

// Log a new trade to the remote influx database. Should only be used in live trading for now.
func (t *TradingEngine) logTrade(trade iex.Order) {
	stateType := "live"
	influx := GetInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{
		"state_type": stateType,
		"side":       strings.ToLower(trade.Side),
	}

	fields := structs.Map(trade)
	fields["algo_name"] = t.Algo.Name
	fields["symbol"] = trade.Symbol

	pt, err := client.NewPoint(
		"trades",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()
}

// Log a new filled trade to the remote influx database. SShould only be used in live trading for now.
func (t *TradingEngine) logFilledTrade(trade iex.Order) {
	stateType := "live"
	influx := GetInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{
		"state_type": stateType,
		"side":       strings.ToLower(trade.Side),
	}

	fields := structs.Map(trade)
	fields["algo_name"] = t.Algo.Name
	fields["symbol"] = trade.Symbol

	pt, err := client.NewPoint(
		"filled_trades",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()
}

// Log the state of the Algo to influx db. Should only be called when live trading for now.
func (t *TradingEngine) logLiveState(test ...bool) {
	stateType := "live"
	if test != nil {
		stateType = "test"
	}

	influx := GetInfluxClient()

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	if err != nil {
		fmt.Println("err", err)
	}

	for symbol, ms := range t.Algo.Account.MarketStates {
		// fmt.Println("logging", symbol, "info")
		tags := map[string]string{
			"state_type": stateType,
			"algo_name":  t.Algo.Name,
			"symbol":     symbol,
		}

		fields := map[string]interface{}{}
		fields["state_type"] = stateType
		fields["Price"] = ms.Bar.Close
		fields["Balance"] = t.Algo.Account.BaseAsset.Quantity
		fields["Quantity"] = ms.Position
		fields["AverageCost"] = ms.AverageCost
		fields["UnrealizedProfit"] = ms.UnrealizedProfit
		fields["Leverage"] = ms.Leverage

		pt, err := client.NewPoint(
			"market",
			tags,
			fields,
			time.Now(),
		)
		if err != nil {
			fmt.Println("err", err)
		}
		bp.AddPoint(pt)

		// params := t.Algo.Params.GetAllParams()
		// pt, err = client.NewPoint(
		// 	"params",
		// 	tags,
		// 	params,
		// 	time.Now(),
		// )
		// if err != nil {
		// 	fmt.Println("err", err)
		// }
		// bp.AddPoint(pt)

		// LOG Options
		// if ms.Info.MarketType == models.Option && ms.Position != 0 {
		// 	tmpTags := tags
		// 	tmpTags["symbol"] = symbol
		// 	o := structs.Map(ms.OptionTheo)
		// 	// Influxdb seems to interpret pointers as strings, need to dereference here
		// 	o["CurrentTime"] = utils.TimeToTimestamp(*ms.OptionTheo.CurrentTime)
		// 	o["UnderlyingPrice"] = *ms.OptionTheo.UnderlyingPrice
		// 	pt1, _ := client.NewPoint(
		// 		"optionTheo",
		// 		tmpTags,
		// 		o,
		// 		time.Now(),
		// 	)
		// 	bp.AddPoint(pt1)

		// 	o = structs.Map(ms)
		// 	// Influxdb seems to interpret pointers as strings, need to dereference here
		// 	o["CurrentTime"] = (*ms.OptionTheo.CurrentTime).String()
		// 	o["UnderlyingPrice"] = *ms.OptionTheo.UnderlyingPrice
		// 	delete(o, "OptionTheo")
		// 	pt2, _ := client.NewPoint(
		// 		"options",
		// 		tmpTags,
		// 		o,
		// 		time.Now(),
		// 	)
		// 	bp.AddPoint(pt2)
		// }

		// LOG orders placed
		// ms.Orders.Range(func(key, value interface{}) bool {
		// 	order := value.(iex.Order)
		// 	if order.Symbol != symbol {
		// 		return false
		// 	}
		// 	fields = map[string]interface{}{
		// 		fmt.Sprintf("%0.2f", order.Rate): order.Amount,
		// 	}

		// 	pt, _ = client.NewPoint(
		// 		"order",
		// 		tags,
		// 		fields,
		// 		time.Now(),
		// 	)
		// 	bp.AddPoint(pt)
		// 	return true
		// })

		if t.Algo.State != nil && len(t.Algo.State) > 0 {
			pt, err := client.NewPoint(
				"state",
				tags,
				t.Algo.State,
				time.Now(),
			)
			if err != nil {
				fmt.Println("Algo.State err", err)
			}
			bp.AddPoint(pt)
		}
	}
	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()
}

// SetLiquidity Set the liquidity available for to buy/sell. IE put 5% of my portfolio on the bid.
func SetLiquidity(algo *models.Algo, marketState *models.MarketState, percentage float64, side int) float64 {
	if marketState.Info.MarketType == models.Future {
		return percentage * algo.Account.BaseAsset.Quantity
	} else {
		if side == 1 {
			return percentage * marketState.Position
		}
		return percentage * marketState.Position * marketState.Bar.Close
	}
}

// CurrentProfit Calculate the current % profit of the position vs
func getCurrentProfit(marketState *models.MarketState, price float64) float64 {
	//TODO this doesnt work on a spot backtest
	if marketState.Position == 0 {
		return 0
	} else if marketState.Position < 0 {
		return utils.CalculateDifference(marketState.AverageCost, price)
	} else {
		return utils.CalculateDifference(price, marketState.AverageCost)
	}
}

// Get the worst-case PNL on the position for a given market state.
func getMaxPositionAbsLoss(algo *models.Algo, marketState *models.MarketState) float64 {
	maxPositionLoss := 0.0
	if algo.Account.ExchangeInfo.DenominatedInQuote {
		if marketState.Position < 0 {
			maxPositionLoss = (marketState.Position * (getCurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
		} else {
			maxPositionLoss = (marketState.Position * (getCurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
		}
	} else {
		if marketState.Position < 0 {
			maxPositionLoss = (algo.Account.BaseAsset.Quantity * (getCurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
		} else {
			maxPositionLoss = (algo.Account.BaseAsset.Quantity * (getCurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
		}
	}

	return maxPositionLoss
}

// Get the PNL on the position for a given market state at close price.
func getPositionAbsProfit(algo *models.Algo, marketState *models.MarketState) float64 {
	positionProfit := 0.0
	if algo.Account.ExchangeInfo.DenominatedInQuote {
		positionProfit = ((math.Abs(marketState.Position) * marketState.Bar.Close) * (getCurrentProfit(marketState, marketState.Bar.Close) * marketState.Leverage))
	} else {
		positionProfit = (algo.Account.BaseAsset.Quantity * (getCurrentProfit(marketState, marketState.Bar.Close) * marketState.Leverage))
	}

	return positionProfit
}

// Get the best-case PNL on the position for a given market state.
func getMaxPositionAbsProfit(algo *models.Algo, marketState *models.MarketState) float64 {
	maxPositionProfit := 0.0
	if algo.Account.ExchangeInfo.DenominatedInQuote {
		if marketState.Position > 0 {
			maxPositionProfit = (math.Abs(marketState.Position) * (getCurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
		} else {
			maxPositionProfit = (math.Abs(marketState.Position) * (getCurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
		}
	} else {
		if marketState.Position > 0 {
			maxPositionProfit = (algo.Account.BaseAsset.Quantity * (getCurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
		} else {
			maxPositionProfit = (algo.Account.BaseAsset.Quantity * (getCurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
		}
	}

	return maxPositionProfit
}

// Log the state of the Algo and update variables like leverage.
func logState(algo *models.Algo, marketState *models.MarketState, timestamp ...time.Time) (state models.History) {
	state = models.History{
		Timestamp:   algo.Timestamp,
		Symbol:      marketState.Symbol,
		Balance:     marketState.Balance,
		Quantity:    marketState.Position,
		AverageCost: marketState.AverageCost,
		Leverage:    marketState.Leverage,
		Profit:      marketState.Profit,
		Weight:      int(marketState.Weight),
		MaxLoss:     getMaxPositionAbsLoss(algo, marketState),
		MaxProfit:   getMaxPositionAbsProfit(algo, marketState),
		Price:       marketState.Bar.Close,
	}

	if marketState.Info.MarketType == models.Future {
		if algo.Account.ExchangeInfo.DenominatedInQuote {
			state.UBalance = ((math.Abs(marketState.Position) * marketState.AverageCost) + marketState.UnrealizedProfit) + marketState.Balance
			state.QuoteBalance = marketState.Balance
		} else {
			state.UBalance = marketState.Balance + marketState.UnrealizedProfit
			state.QuoteBalance = (marketState.Balance + marketState.UnrealizedProfit) * marketState.Bar.Close
		}
	} else {
		state.UBalance = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Account.BaseAsset.Quantity*marketState.Bar.Close+(marketState.Position), 0.0, algo.Account.BaseAsset.Quantity, marketState.Position, marketState.Bar.Close, marketState.AverageCost))
	}
	return
}

// Get the remote influx db client for logging live trading data.
func GetInfluxClient() client.Client {
	influxURL := os.Getenv("YANTRA_LIVE_DB_URL")
	if influxURL == "" {
		log.Fatalln("You need to set the `YANTRA_LIVE_DB_URL` env variable")
	}

	influxUser := os.Getenv("YANTRA_LIVE_DB_USER")
	influxPassword := os.Getenv("YANTRA_LIVE_DB_PASSWORD")

	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     influxURL,
		Username: influxUser,
		Password: influxPassword,
	})

	if err != nil {
		fmt.Println("err", err)
	}

	return influx
}

// Log live backtest results for a given algo.
func logBacktest(algo *models.Algo) {
	influxURL := os.Getenv("YANTRA_BACKTEST_DB_URL")
	if influxURL == "" {
		log.Fatalln("You need to set the `YANTRA_BACKTEST_DB_URL` env variable")
	}

	influxUser := os.Getenv("YANTRA_BACKTEST_DB_USER")
	influxPassword := os.Getenv("YANTRA_BACKTEST_DB_PASSWORD")

	influx, _ := client.NewHTTPClient(client.HTTPConfig{
		Addr:     influxURL,
		Username: influxUser,
		Password: influxPassword,
		Timeout:  (time.Millisecond * 1000 * 10),
	})

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "backtests",
		Precision: "us",
	})

	// uuid := algo.Name + "-" + uuid.New().String()
	tags := map[string]string{}
	fields := structs.Map(algo.Result)
	fields["algo_name"] = algo.Name

	pt, _ := client.NewPoint(
		"result",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	err := client.Client.Write(influx, bp)
	log.Println(algo.Name, err)

	influx.Close()
}

// Create a Spread on the bid/ask, this fuction is used to create an arrary of orders that spreads across the order book.
func CreateSpread(algo *models.Algo, marketState *models.MarketState, weight int32, confidence float64, price float64, spread float64) models.OrderArray {
	tickSize := marketState.Info.PricePrecision
	maxOrders := marketState.Info.MaxOrders
	xStart := 0.0
	if weight == 1 {
		xStart = price - (price * spread)
	} else {
		xStart = price
	}
	xStart = utils.Round(xStart, tickSize)

	xEnd := xStart + (xStart * spread)
	xEnd = utils.Round(xEnd, tickSize)

	diff := xEnd - xStart

	if diff/tickSize >= float64(maxOrders) {
		newTickSize := diff / (float64(maxOrders) - 1)
		tickSize = utils.Round(newTickSize, tickSize)
	}

	var priceArr []float64

	if weight == 1 {
		priceArr = utils.Arange(xStart, xEnd-float64(tickSize), float64(tickSize))
	} else {
		if xStart-xEnd < float64(tickSize) {
			xEnd = xEnd + float64(tickSize)
		}
		priceArr = utils.Arange(xStart, xEnd, float64(tickSize))
	}

	temp := utils.DivArr(priceArr, xStart)

	dist := utils.ExpArr(temp, confidence)
	normalizer := 1 / utils.SumArr(dist)
	orderArr := utils.MulArr(dist, normalizer)
	if weight == 1 {
		orderArr = utils.ReverseArr(orderArr)
	}
	return models.OrderArray{Price: priceArr, Quantity: orderArr}
}
