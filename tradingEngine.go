package yantra

import (
	"fmt"
	"log"
	"math"
	"os"
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

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
)

type TradingEngine struct {
	Algo                 *models.Algo
	firstTrade           bool
	firstPositionUpdate  bool
	commitHash           string
	lastTest             int64
	lastWalletSync       int64
	startTime            time.Time
	endTime              time.Time
	theoEngine           *te.TheoEngine
	lastContractUpdate   int
	contractUpdatePeriod int
}

func NewTradingEngine(Algo *models.Algo, contractUpdatePeriod int) TradingEngine {
	//TODO: should theo engine and other vars be initialized here?
	return TradingEngine{
		Algo:                 Algo,
		firstTrade:           true,
		firstPositionUpdate:  true,
		commitHash:           time.Now().String(),
		lastTest:             0,
		lastWalletSync:       0,
		startTime:            time.Now(),
		theoEngine:           nil,
		lastContractUpdate:   0,
		contractUpdatePeriod: contractUpdatePeriod,
	}
}

func (t *TradingEngine) RunTest(start time.Time, end time.Time, rebalance func(*models.Algo), setupData func(*models.Algo)) {
	exchangeVars := iex.ExchangeConf{
		Exchange:       t.Algo.Account.ExchangeInfo.Exchange,
		ServerUrl:      t.Algo.Account.ExchangeInfo.ExchangeURL,
		AccountID:      "test",
		OutputResponse: false,
	}
	mockExchange := tantra.NewTest(exchangeVars, &t.Algo.Account, start, end, t.Algo.DataLength)
	barData := t.LoadBarData(t.Algo, start, end)
	for symbol, data := range barData {
		logger.Infof("Loaded %v instances of bar data for %v with start %v and end %v.\n", len(data), symbol, start, end)
	}
	t.SetAlgoCandleData(barData)
	mockExchange.SetCandleData(barData)
	mockExchange.SetCurrentTime(start)
	t.Algo.Client = mockExchange
	t.Algo.Timestamp = start
	t.endTime = end
	t.Connect("", false, rebalance, setupData, true)
}

func (t *TradingEngine) SetAlgoCandleData(candleData map[string][]*models.Bar) {
	for symbol, data := range candleData {
		marketState, ok := t.Algo.Account.MarketStates[symbol]
		if !ok {
			logger.Errorf("Cannot set bar data for market state %v.\n", symbol)
		}
		logger.Infof("Setting candle data for %v with %v bars.\n", symbol, len(data))
		var ohlcv models.OHLCV
		numBars := len(data)
		ohlcv.Timestamp = make([]int64, numBars)
		ohlcv.Open = make([]float64, numBars)
		ohlcv.Low = make([]float64, numBars)
		ohlcv.High = make([]float64, numBars)
		ohlcv.Close = make([]float64, numBars)
		ohlcv.Volume = make([]float64, numBars)
		for i, candle := range data {
			ohlcv.Timestamp[i] = candle.Timestamp
			ohlcv.Open[i] = candle.Open
			ohlcv.High[i] = candle.High
			ohlcv.Low[i] = candle.Low
			ohlcv.Close[i] = candle.Close
			ohlcv.Volume[i] = candle.Volume
		}
		marketState.OHLCV = ohlcv
	}
}

func (t *TradingEngine) InsertNewCandle(candle iex.TradeBin) {
	marketState, ok := t.Algo.Account.MarketStates[candle.Symbol]
	if !ok {
		logger.Errorf("Cannot insert new candle for symbol %v (candle=%v)\n", candle.Symbol, candle)
		return
	}
	ohlcv := marketState.OHLCV
	ohlcv.Timestamp = append(ohlcv.Timestamp, int64(utils.TimeToTimestamp(candle.Timestamp)))
	ohlcv.Open = append(ohlcv.Open, candle.Open)
	ohlcv.High = append(ohlcv.High, candle.High)
	ohlcv.Low = append(ohlcv.Low, candle.Low)
	ohlcv.Close = append(ohlcv.Close, candle.Close)
	ohlcv.Volume = append(ohlcv.Volume, candle.Volume)
	marketState.OHLCV = ohlcv
	t.Algo.Index = len(ohlcv.Timestamp) - 1
}

func (t *TradingEngine) LoadBarData(Algo *models.Algo, start time.Time, end time.Time) map[string][]*models.Bar {
	barData := make(map[string][]*models.Bar)
	for symbol, marketState := range Algo.Account.MarketStates {
		logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", symbol, Algo.RebalanceInterval, Algo.DataLength+1)
		// TODO handle extra bars to account for dataLength here
		// barData[symbol] = database.GetData(symbol, Algo.Account.ExchangeInfo.Exchange, Algo.RebalanceInterval, Algo.DataLength+100)
		barData[symbol] = database.GetCandlesByTime(symbol, Algo.Account.ExchangeInfo.Exchange, Algo.RebalanceInterval, start, end, Algo.DataLength)
		Algo.Index = Algo.DataLength
		marketState.Bar = *barData[symbol][len(barData[symbol])-1]
		marketState.LastPrice = marketState.Bar.Close
		logger.Infof("Initialized bar for %v: %v\n", symbol, marketState.Bar)
	}
	return barData
}

// Connect is called to connect to an exchange's WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of models.Algo.RebalanceInterval
// This is intentional, look at models.Algo.AutoOrderPlacement to understand this paradigm.
func (t *TradingEngine) Connect(settingsFileName string, secret bool, rebalance func(*models.Algo), setupData func(*models.Algo), test ...bool) {
	utils.LoadENV(secret)
	var isTest bool
	if test != nil {
		isTest = test[0]
		database.Setup()
	} else {
		isTest = false
		database.Setup("remote")
	}
	if t.Algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}

	var err error
	var config models.Secret

	if !isTest {
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
		if err != nil {
			logger.Error(err)
		}
	}

	//TODO do we need this order status?
	// t.orderStatus = Algo.Client.GetPotentialOrderStatus()

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
			log.Fatal("Could not find underlying market for options exchange %v\n", t.Algo.Account.ExchangeInfo.Exchange)
		}
		t.Algo.Account.MarketStates[underlyingMarket.Symbol] = underlyingMarket
		logger.Infof("Initialized underlying market: %v\n", underlyingMarket.Symbol)
		theoEngine := te.NewTheoEngine(underlyingMarket, &t.Algo.Timestamp, 60000, 86400000, 0, 0, t.Algo.LogLevel)
		t.Algo.TheoEngine = &theoEngine
		t.theoEngine = &theoEngine
		logger.Infof("Built new theo engine.\n")
		if isTest {
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
	for marketSymbol, marketState := range t.Algo.Account.MarketStates {
		if marketState.Info.MarketType == models.Future || marketState.Info.MarketType == models.Spot {
			symbol := strings.ToLower(marketSymbol)
			//Ordering is important, get wallet and position first then market info
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_WALLET, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_ORDER, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_POSITION, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol})
		}
	}

	logger.Infof("Subscribing to %v channels.\n", len(subscribeInfos))

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan: make(chan []iex.WsPosition, 2),
		TradeBinChan: make(chan []iex.TradeBin, 2),
		WalletChan:   make(chan *iex.WSWallet, 2),
		OrderChan:    make(chan []iex.Order, 2),
	}

	// Start the websocket.
	err = t.Algo.Client.StartWS(&iex.WsConfig{
		Host:      t.Algo.Account.ExchangeInfo.WSStream,
		Streams:   subscribeInfos,
		Channels:  channels,
		ApiSecret: config.APISecret,
		ApiKey:    config.APIKey,
	})

	logger.Infof("Started client order channel: %v\n", channels.OrderChan)
	logger.Infof("Started client trade channel: %v\n", channels.TradeBinChan)

	if err != nil {
		msg := fmt.Sprintf("Error starting websockets: %v\n", err)
		log.Fatal(msg)
	}

	// All of these channels send themselves back so that the test can wait for each individual to complete
	for {
		select {
		case positions := <-channels.PositionChan:
			t.updatePositions(t.Algo, positions)
			channels.PositionChan <- positions
		case trades := <-channels.TradeBinChan:
			startTimestamp := time.Now().UnixNano()
			logger.Infof("Recieved %v new trade updates: %v\n", len(trades), trades)
			// Update active contracts if we are trading options
			if t.theoEngine != nil {
				t.UpdateActiveContracts()
				t.UpdateMidMarketPrices()
				t.theoEngine.ScanOptions(false, true)
			} else {
				logger.Debugf("Cannot update active contracts, theo engine is nil\n")
			}
			// Update your local bars
			for _, trade := range trades {
				t.InsertNewCandle(trade)
				marketState, _ := t.Algo.Account.MarketStates[trade.Symbol]
				// t.updateBars(t.Algo, trade)
				// now fetch the bars
				// bars := database.GetData(trade.Symbol, t.Algo.Account.ExchangeInfo.Exchange, t.Algo.RebalanceInterval, t.Algo.DataLength+100)
				// Did we get enough data to run this? If we didn't then throw fatal error to notify system
				if t.Algo.DataLength < len(marketState.OHLCV.Timestamp) {
					t.updateState(t.Algo, trade.Symbol, setupData)
					rebalance(t.Algo)
				} else {
					log.Fatalln("Not enough trade data. (local data length", len(marketState.OHLCV.Timestamp), "data length wanted by Algo", t.Algo.DataLength, ")")
				}
			}
			for _, marketState := range t.Algo.Account.MarketStates {
				logState(t.Algo, marketState)
				if !isTest {
					t.logLiveState(marketState)
					t.runTest(t.Algo, setupData, rebalance)
					t.checkWalletHistory(t.Algo, settingsFileName)
				}
			}
			t.aggregateAccountProfit()
			logger.Debugf("Trade processing took %v ns\n", time.Now().UnixNano()-startTimestamp)
			channels.TradeBinChan <- trades
			if !t.Algo.Timestamp.Before(t.endTime) {
				logger.Infof("Algo timestamp %v past end time %v, killing trading engine.\n", t.Algo.Timestamp, t.endTime)
				return
			}
		case newOrders := <-channels.OrderChan:
			startTimestamp := time.Now().UnixNano()
			logger.Infof("Recieved %v new order updates\n", len(newOrders))
			// TODO look at the response for a market order, does it send 2 orders filled and placed or just filled
			t.updateOrders(t.Algo, newOrders, true)
			// TODO callback to order function
			channels.OrderChan <- newOrders
			logger.Infof("Order processing took %v ns\n", time.Now().UnixNano()-startTimestamp)
		case update := <-channels.WalletChan:
			t.updateAlgoBalances(t.Algo, update.Balance)
			channels.WalletChan <- update
		}
		if channels.TradeBinChan == nil {
			logger.Errorf("Trade bin channel is nil, breaking...\n")
			break
		}
	}
	logger.Infof("Reached end of connect.\n")
}

func (t *TradingEngine) checkWalletHistory(Algo *models.Algo, settingsFileName string) {
	timeSinceLastSync := database.GetBars()[Algo.Index].Timestamp - t.lastWalletSync
	if timeSinceLastSync > (60 * 60 * 60) {
		logger.Info("It has been", timeSinceLastSync, "seconds since the last wallet history download, fetching latest deposits and withdrawals.")
		t.lastWalletSync = database.GetBars()[Algo.Index].Timestamp
		walletHistory, err := Algo.Client.GetWalletHistory(Algo.Account.BaseAsset.Symbol)
		if err != nil {
			logger.Error("There was an error fetching the wallet history", err)
		} else {
			if len(walletHistory) > 0 {
				database.LogWalletHistory(Algo, settingsFileName, walletHistory)
			}
		}
	}
}

// Inject orders directly into market state upon update
func (t *TradingEngine) updateOrders(Algo *models.Algo, orders []iex.Order, isUpdate bool) {
	logger.Infof("Processing %v order updates.\n", len(orders))
	if isUpdate {
		// Add to existing order state
		for _, newOrder := range orders {
			logger.Debugf("Processing order update: %v\n", newOrder)
			marketState, ok := Algo.Account.MarketStates[newOrder.Market]
			if !ok {
				logger.Errorf("New order symbol %v not found in account market states\n", newOrder.Market)
				continue
			}
			marketState.Orders.Store(newOrder.OrderID, newOrder)
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
		for symbol, marketState := range Algo.Account.MarketStates {
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

// TODO do we just want to do a tantra test here?
func (t *TradingEngine) runTest(Algo *models.Algo, setupData func(*models.Algo), rebalance func(*models.Algo)) {
	if t.lastTest != database.GetBars()[Algo.Index].Timestamp {
		t.lastTest = database.GetBars()[Algo.Index].Timestamp
		testAlgo := models.Algo{}
		copier.Copy(&testAlgo, &Algo)
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

func (t *TradingEngine) updatePositions(Algo *models.Algo, positions []iex.WsPosition) {
	logger.Info("Position Update:", positions)
	if len(positions) > 0 {
		for _, position := range positions {
			if position.Symbol == Algo.Account.BaseAsset.Symbol {
				Algo.Account.BaseAsset.Quantity = position.CurrentQty
				logger.Infof("Updated base asset %v: %v\n", Algo.Account.BaseAsset.Symbol, Algo.Account.BaseAsset.Quantity)
			} else {
				marketState, ok := Algo.Account.MarketStates[position.Symbol]
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
				if t.firstPositionUpdate {
					marketState.ShouldHaveQuantity = marketState.Position
				}
				logState(Algo, marketState)
			}
		}
	}
	t.firstPositionUpdate = false
}

func (t *TradingEngine) aggregateAccountProfit() {
	totalUnrealizedProfit := 0.
	totalRealizedProfit := 0.
	for _, marketState := range t.Algo.Account.MarketStates {
		totalUnrealizedProfit += marketState.UnrealizedProfit
		totalRealizedProfit += marketState.RealizedProfit
	}
	t.Algo.Account.UnrealizedProfit = totalUnrealizedProfit
	t.Algo.Account.RealizedProfit = totalRealizedProfit
	t.Algo.Account.Profit = totalUnrealizedProfit + totalRealizedProfit
	logger.Debugf("Aggregated account unrealized profit: %v, realized profit: %v, total profit: %v\n",
		t.Algo.Account.UnrealizedProfit, t.Algo.Account.RealizedProfit, t.Algo.Account.Profit)
}

func (t *TradingEngine) updateAlgoBalances(Algo *models.Algo, balances []iex.WSBalance) {
	for _, updatedBalance := range balances {
		balance, ok := Algo.Account.Balances[updatedBalance.Asset]
		if ok {
			balance.Quantity = updatedBalance.Balance
		} else {
			// If unknown asset, create a new asset
			newAsset := models.Asset{
				Symbol:   updatedBalance.Asset,
				Quantity: updatedBalance.Balance,
			}
			Algo.Account.Balances[updatedBalance.Asset] = &newAsset
			logger.Infof("New balance found: %v\n", Algo.Account.Balances[updatedBalance.Asset])
		}
		if updatedBalance.Asset == Algo.Account.BaseAsset.Symbol {
			Algo.Account.BaseAsset.Quantity = updatedBalance.Balance
			logger.Infof("Updated base asset quantity: %v\n", Algo.Account.BaseAsset.Quantity)
		} else if Algo.Account.ExchangeInfo.Spot {
			// This could be a spot position update, in which case we should update the respective market state's position
			for symbol, marketState := range Algo.Account.MarketStates {
				if marketState.Info.MarketType == models.Spot && marketState.Info.QuoteSymbol == updatedBalance.Asset {
					marketState.Position = updatedBalance.Balance
					logger.Infof("Updated position for spot market %v: %v\n", symbol, marketState.Position)
				}
			}
		}
	}
}

func (t *TradingEngine) updateBars(Algo *models.Algo, trade iex.TradeBin) {
	if Algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		diff := trade.Timestamp.Sub(time.Unix(database.GetBars()[Algo.Index].Timestamp/1000, 0))
		if diff.Minutes() >= 60 {
			database.UpdateBars(Algo.Client, trade.Symbol, Algo.RebalanceInterval, 1)
		}
	} else if Algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		database.UpdateBars(Algo.Client, trade.Symbol, Algo.RebalanceInterval, 1)
	} else {
		log.Fatal("This rebalance interval is not supported")
	}
	Algo.Index = len(database.GetBars()) - 1
	logger.Info("Time Elapsed", t.startTime.Sub(time.Now()), "Index", Algo.Index)
}

func (t *TradingEngine) updateState(Algo *models.Algo, symbol string, setupData func(*models.Algo)) {
	marketState, ok := Algo.Account.MarketStates[symbol]
	if !ok {
		logger.Errorf("Cannot update state for %v (could not find market state).\n", symbol)
		return
	}
	setupData(Algo)
	lastCandleIndex := len(marketState.OHLCV.Timestamp) - 1
	// TODO initialize vwap, quote volume?
	marketState.Bar = models.Bar{
		Timestamp: marketState.OHLCV.Timestamp[lastCandleIndex],
		Open:      marketState.OHLCV.Open[lastCandleIndex],
		High:      marketState.OHLCV.High[lastCandleIndex],
		Low:       marketState.OHLCV.Low[lastCandleIndex],
		Close:     marketState.OHLCV.Close[lastCandleIndex],
		Volume:    marketState.OHLCV.Volume[lastCandleIndex],
	}
	Algo.Timestamp = time.Unix(marketState.Bar.Timestamp/1000, 0).UTC()
	marketState.LastPrice = marketState.Bar.Close
	// logger.Info("Algo.Timestamp", Algo.Timestamp, "Algo.Index", Algo.Index, "Close Price", Algo.Market.Price.Close)
	if t.firstTrade {
		logState(Algo, marketState)
		t.firstTrade = false
	}
}

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
					marketState = models.NewMarketStateFromInfo(marketInfo, &t.Algo.Account.BaseAsset.Quantity)
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

// Delete all expired options without profit values to conserve time and space resources
func (t *TradingEngine) RemoveExpiredOptions() {
	numOptions := len(t.theoEngine.Options)
	for symbol, option := range t.theoEngine.Options {
		if option.Info.Expiry <= t.theoEngine.GetCurrentTimestamp() {
			option.Status = models.Expired
		}
		if option.Status == models.Expired && option.Profit == 0. {
			delete(t.theoEngine.Options, symbol)
			//TODO delete from Algo.Accounts here or keep for history?
			logger.Infof("Removed expired option: %v\n", symbol)
		}
	}
	logger.Infof("Removed %v expired option contracts; %v contracts remain.\n", numOptions-len(t.theoEngine.Options), len(t.theoEngine.Options))
	t.theoEngine.UpdateOptionIndexes()
}

func (t *TradingEngine) logTrade(trade iex.Order) {
	stateType := "live"
	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "Algos",
		Precision: "us",
	})

	tags := map[string]string{"Algo_name": t.Algo.Name, "commit_hash": t.commitHash, "state_type": stateType, "side": strings.ToLower(trade.Side)}

	fields := structs.Map(trade)
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

func (t *TradingEngine) logFilledTrade(trade iex.Order) {
	stateType := "live"
	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "Algos",
		Precision: "us",
	})

	tags := map[string]string{"Algo_name": t.Algo.Name, "commit_hash": t.commitHash, "state_type": stateType, "side": strings.ToLower(trade.Side)}

	fields := structs.Map(trade)
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

//Log the state of the Algo to influx db
func (t *TradingEngine) logLiveState(marketState *models.MarketState, test ...bool) {
	stateType := "live"
	if test != nil {
		stateType = "test"
	}

	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "Algos",
		Precision: "us",
	})

	tags := map[string]string{"Algo_name": t.Algo.Name, "commit_hash": t.commitHash, "state_type": stateType}

	fields := structs.Map(marketState)

	//TODO: shouldn't have to manually delete Options param here
	_, ok := fields["Options"]
	if ok {
		delete(fields, "Options")
	}

	fields["Price"] = marketState.Bar.Close
	fields["Balance"] = t.Algo.Account.BaseAsset.Quantity
	fields["Quantity"] = marketState.Position

	pt, err := client.NewPoint(
		"market",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	fields = t.Algo.Params[marketState.Symbol]

	if marketState.AutoOrderPlacement {
		fields["EntryOrderSize"] = marketState.EntryOrderSize
		fields["ExitOrderSize"] = marketState.ExitOrderSize
		fields["DeleverageOrderSize"] = marketState.DeleverageOrderSize
		fields["LeverageTarget"] = marketState.LeverageTarget
		fields["ShouldHaveQuantity"] = marketState.ShouldHaveQuantity
		fields["FillPrice"] = marketState.FillPrice
	}

	pt, err = client.NewPoint(
		"params",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	// LOG Options
	for symbol, option := range t.Algo.Account.MarketStates {
		if option.Info.MarketType == models.Option && option.Position != 0 {
			tmpTags := tags
			tmpTags["symbol"] = symbol
			o := structs.Map(option.OptionTheo)
			// Influxdb seems to interpret pointers as strings, need to dereference here
			o["CurrentTime"] = utils.TimeToTimestamp(*option.OptionTheo.CurrentTime)
			o["UnderlyingPrice"] = *option.OptionTheo.UnderlyingPrice
			pt1, _ := client.NewPoint(
				"optionTheo",
				tmpTags,
				o,
				time.Now(),
			)
			bp.AddPoint(pt1)

			o = structs.Map(option)
			// Influxdb seems to interpret pointers as strings, need to dereference here
			o["CurrentTime"] = (*option.OptionTheo.CurrentTime).String()
			o["UnderlyingPrice"] = *option.OptionTheo.UnderlyingPrice
			delete(o, "OptionTheo")
			pt2, _ := client.NewPoint(
				"options",
				tmpTags,
				o,
				time.Now(),
			)
			bp.AddPoint(pt2)
		}
	}

	// LOG orders placed
	marketState.Orders.Range(func(key, value interface{}) bool {
		order := value.(iex.Order)
		fields = map[string]interface{}{
			fmt.Sprintf("%0.2f", order.Rate): order.Amount,
		}

		pt, err = client.NewPoint(
			"order",
			tags,
			fields,
			time.Now(),
		)
		bp.AddPoint(pt)
		return true
	})

	if t.Algo.State != nil {
		pt, err := client.NewPoint(
			"state",
			tags,
			t.Algo.State,
			time.Now(),
		)
		if err != nil {
			log.Fatal(err)
		}
		bp.AddPoint(pt)
	}
	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()
}

// SetLiquidity Set the liquidity available for to buy/sell. IE put 5% of my portfolio on the bid.
func SetLiquidity(Algo *models.Algo, marketState *models.MarketState, percentage float64, side int) float64 {
	if marketState.Info.MarketType == models.Future {
		return percentage * Algo.Account.BaseAsset.Quantity
	} else {
		if side == 1 {
			return percentage * marketState.Position
		}
		return percentage * marketState.Position * marketState.Bar.Close
	}
}

// CurrentProfit Calculate the current % profit of the position vs
func CurrentProfit(marketState *models.MarketState, price float64) float64 {
	//TODO this doesnt work on a spot backtest
	if marketState.Position == 0 {
		return 0
	} else if marketState.Position < 0 {
		return utils.CalculateDifference(marketState.AverageCost, price)
	} else {
		return utils.CalculateDifference(price, marketState.AverageCost)
	}
}

func getPositionAbsLoss(Algo *models.Algo, marketState *models.MarketState) float64 {
	positionLoss := 0.0
	if marketState.Position < 0 {
		positionLoss = (Algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
	} else {
		positionLoss = (Algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
	}
	return positionLoss
}

func getPositionAbsProfit(Algo *models.Algo, marketState *models.MarketState) float64 {
	positionProfit := 0.0
	if marketState.Position > 0 {
		positionProfit = (Algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
	} else {
		positionProfit = (Algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
	}
	return positionProfit
}

func getExitOrderSize(marketState *models.MarketState, orderSizeGreaterThanPositionSize bool) float64 {
	if orderSizeGreaterThanPositionSize {
		return marketState.Leverage
	} else {
		return marketState.ExitOrderSize
	}
}

func getEntryOrderSize(marketState *models.MarketState, orderSizeGreaterThanMaxPositionSize bool) float64 {
	if orderSizeGreaterThanMaxPositionSize {
		return marketState.LeverageTarget - marketState.Leverage //-marketState.LeverageTarget
	} else {
		return marketState.EntryOrderSize
	}
}

func canBuy(Algo *models.Algo, marketState *models.MarketState) float64 {
	if marketState.CanBuyBasedOnMax {
		return (Algo.Account.BaseAsset.Quantity * marketState.Bar.Open) * marketState.MaxLeverage
	} else {
		return (Algo.Account.BaseAsset.Quantity * marketState.Bar.Open) * marketState.LeverageTarget
	}
}

//Log the state of the Algo and update variables like leverage
func logState(Algo *models.Algo, marketState *models.MarketState, timestamp ...time.Time) (state models.History) {
	// Algo.History.Timestamp = append(Algo.History.Timestamp, timestamp)
	var balance float64
	if marketState.Info.MarketType == models.Future {
		balance = Algo.Account.BaseAsset.Quantity
		marketState.Leverage = math.Abs(marketState.Position) / (marketState.Bar.Close * balance)
	} else {
		if marketState.AverageCost == 0 {
			marketState.AverageCost = marketState.Bar.Close
		}
		balance = (Algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		marketState.Leverage = (Algo.Account.BaseAsset.Quantity * marketState.Bar.Close) / balance
		// log.Println("BaseAsset Quantity", Algo.Account.BaseAsset.Quantity, "QuoteAsset Value", marketState.Position/marketState.Bar)
		// log.Println("BaseAsset Value", Algo.Account.BaseAsset.Quantity*models.MarketState.Bar, "QuoteAsset Quantity", marketState.Position)
		// log.Println("Leverage", marketState.Leverage)
	}

	// fmt.Println(Algo.Timestamp, "Funds", Algo.Account.BaseAsset.Quantity, "Quantity", marketState.Position)
	// fmt.Println(Algo.Timestamp, Algo.Account.BaseAsset.Quantity, Algo.CurrentProfit(marketState.Bar))
	// marketState.Profit = (Algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Close) * marketState.Leverage))
	// fmt.Println(Algo.Timestamp, marketState.Profit)
	marketState.Profit = marketState.UnrealizedProfit + marketState.RealizedProfit

	if timestamp != nil {
		Algo.Timestamp = timestamp[0]
		state = models.History{
			Timestamp:   Algo.Timestamp.String(),
			Balance:     balance,
			Quantity:    marketState.Position,
			AverageCost: marketState.AverageCost,
			Leverage:    marketState.Leverage,
			Profit:      marketState.Profit,
			Weight:      int(marketState.Weight),
			MaxLoss:     getPositionAbsLoss(Algo, marketState),
			MaxProfit:   getPositionAbsProfit(Algo, marketState),
			Price:       marketState.Bar.Close,
		}

		if marketState.Info.MarketType == models.Future {
			if math.IsNaN(marketState.Profit) {
				state.UBalance = balance
			} else {
				state.UBalance = balance + marketState.Profit
			}
		} else {
			state.UBalance = (Algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		}
	}
	if Algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", Algo.Account.BaseAsset.Quantity*marketState.Bar.Close+(marketState.Position), 0.0, Algo.Account.BaseAsset.Quantity, marketState.Position, marketState.Bar.Close, marketState.AverageCost))
	}
	return
}

func getOrderSize(Algo *models.Algo, marketState *models.MarketState, currentPrice float64, live ...bool) (orderSize float64, side float64) {
	currentWeight := math.Copysign(1, marketState.Position)
	if marketState.Position == 0 {
		currentWeight = float64(marketState.Weight)
	}
	adding := currentWeight == float64(marketState.Weight)
	// fmt.Printf("CURRENT WEIGHT %v, adding %v, leverage target %v, can buy %v, deleverage order size %v\n", currentWeight, adding, marketState.LeverageTarget, canBuy(Algo), marketState.DeleverageOrderSize)
	// fmt.Printf("Getting order size with quote asset quantity: %v\n", marketState.Position)

	// Change order sizes for live to ensure similar boolen checks
	exitOrderSize := marketState.ExitOrderSize
	entryOrderSize := marketState.EntryOrderSize
	deleverageOrderSize := marketState.DeleverageOrderSize

	if live != nil && live[0] {
		if Algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
			exitOrderSize = marketState.ExitOrderSize / 60
			entryOrderSize = marketState.EntryOrderSize / 60
			deleverageOrderSize = marketState.DeleverageOrderSize / 60
		}
	}

	if (currentWeight == 0 || adding) && marketState.Leverage+marketState.DeleverageOrderSize <= marketState.LeverageTarget && marketState.Weight != 0 {
		// fmt.Printf("Getting entry order with entry order size %v, leverage target %v, leverage %v\n", entryOrderSize, marketState.LeverageTarget, marketState.Leverage)
		orderSize = getEntryOrderSize(marketState, entryOrderSize > marketState.LeverageTarget-marketState.Leverage)
		side = float64(marketState.Weight)
	} else if !adding {
		// fmt.Printf("Getting exit order size with exit order size %v, leverage %v, weight %v\n", exitOrderSize, marketState.Leverage, marketState.Weight)
		orderSize = getExitOrderSize(marketState, exitOrderSize > marketState.Leverage && marketState.Weight == 0)
		side = float64(currentWeight * -1)
	} else if math.Abs(marketState.Position) > canBuy(Algo, marketState)*(1+deleverageOrderSize) && adding {
		orderSize = marketState.DeleverageOrderSize
		side = float64(currentWeight * -1)
	} else if marketState.Weight == 0 && marketState.Leverage > 0 {
		orderSize = getExitOrderSize(marketState, exitOrderSize > marketState.Leverage)
		//side = Opposite of the quantity
		side = -math.Copysign(1, marketState.Position)
	} else if canBuy(Algo, marketState) > math.Abs(marketState.Position) {
		// If I can buy more, place order to fill diff of canBuy and current quantity
		orderSize = utils.CalculateDifference(canBuy(Algo, marketState), math.Abs(marketState.Position))
		side = float64(marketState.Weight)
	}
	return
}

func getFillPrice(Algo *models.Algo, marketState *models.MarketState) float64 {
	var fillPrice float64
	if Algo.FillType == exchanges.FillType().Worst {
		if marketState.Weight > 0 && marketState.Position > 0 {
			fillPrice = marketState.Bar.High
		} else if marketState.Weight < 0 && marketState.Position < 0 {
			fillPrice = marketState.Bar.Low
		} else if marketState.Weight != 1 && marketState.Position > 0 {
			fillPrice = marketState.Bar.Low
		} else if marketState.Weight != -1 && marketState.Position < 0 {
			fillPrice = marketState.Bar.High
		} else {
			fillPrice = marketState.Bar.Close
		}
	} else if Algo.FillType == exchanges.FillType().Close {
		fillPrice = marketState.Bar.Close
	} else if Algo.FillType == exchanges.FillType().Open {
		fillPrice = marketState.Bar.Open
	} else if Algo.FillType == exchanges.FillType().MeanOC {
		fillPrice = (marketState.Bar.Open + marketState.Bar.Close) / 2
	} else if Algo.FillType == exchanges.FillType().MeanHL {
		fillPrice = (marketState.Bar.High + marketState.Bar.Low) / 2
	}
	return fillPrice
}

func getInfluxClient() client.Client {
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

// CreateSpread Create a Spread on the bid/ask, this fuction is used to create an arrary of orders that spreads across the order book.
func CreateSpread(Algo *models.Algo, marketState *models.MarketState, weight int32, confidence float64, price float64, spread float64) models.OrderArray {
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
