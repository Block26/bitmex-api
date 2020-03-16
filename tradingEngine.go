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
	algo                 *models.Algo
	firstTrade           bool
	firstPositionUpdate  bool
	commitHash           string
	lastTest             int64
	lastWalletSync       int64
	startTime            time.Time
	theoEngine           *te.TheoEngine
	lastContractUpdate   int
	contractUpdatePeriod int
}

func NewTradingEngine(algo *models.Algo, contractUpdatePeriod int) TradingEngine {
	//TODO: should theo engine and other vars be initialized here?
	return TradingEngine{
		algo:                 algo,
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
		Exchange:       t.algo.Account.ExchangeInfo.Exchange,
		ServerUrl:      t.algo.Account.ExchangeInfo.ExchangeURL,
		AccountID:      "test",
		OutputResponse: false,
	}
	mockExchange := tantra.NewTest(exchangeVars, &t.algo.Account, start, end, t.algo.DataLength)
	barData := t.LoadBarData(t.algo, start, end)
	for symbol, data := range barData {
		logger.Infof("Loaded %v instances of bar data for %v with start %v and end %v.\n", len(data), symbol, start, end)
	}
	t.SetAlgoCandleData(barData)
	mockExchange.SetCandleData(barData)
	mockExchange.SetCurrentTime(start)
	t.algo.Client = mockExchange
	t.algo.Timestamp = start
	t.Connect("", false, rebalance, setupData, true)
}

func (t *TradingEngine) SetAlgoCandleData(candleData map[string][]*models.Bar) {
	for symbol, data := range candleData {
		marketState, ok := t.algo.Account.MarketStates[symbol]
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
	marketState, ok := t.algo.Account.MarketStates[candle.Symbol]
	if !ok {
		logger.Errorf("Cannot insert new candle for symbol %v\n", candle.Symbol)
	}
	ohlcv := marketState.OHLCV
	ohlcv.Timestamp = append(ohlcv.Timestamp, int64(utils.TimeToTimestamp(candle.Timestamp)))
	ohlcv.Open = append(ohlcv.Open, candle.Open)
	ohlcv.High = append(ohlcv.High, candle.High)
	ohlcv.Low = append(ohlcv.Low, candle.Low)
	ohlcv.Close = append(ohlcv.Close, candle.Close)
	ohlcv.Volume = append(ohlcv.Volume, candle.Volume)
	marketState.OHLCV = ohlcv
	t.algo.Index = len(ohlcv.Timestamp) - 1
}

func (t *TradingEngine) LoadBarData(algo *models.Algo, start time.Time, end time.Time) map[string][]*models.Bar {
	barData := make(map[string][]*models.Bar)
	for symbol, marketState := range algo.Account.MarketStates {
		logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", symbol, algo.RebalanceInterval, algo.DataLength+1)
		// TODO handle extra bars to account for dataLength here
		// barData[symbol] = database.GetData(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, algo.DataLength+100)
		barData[symbol] = database.GetCandlesByTime(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, start, end, algo.DataLength)
		algo.Index = algo.DataLength
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
	if t.algo.RebalanceInterval == "" {
		log.Fatal("RebalanceInterval must be set")
	}

	var err error
	var config models.Secret

	if !isTest {
		config = utils.LoadSecret(settingsFileName, secret)
		logger.Info("Loaded config for", t.algo.Account.ExchangeInfo.Exchange, "secret", settingsFileName)
		exchangeVars := iex.ExchangeConf{
			Exchange:       t.algo.Account.ExchangeInfo.Exchange,
			ServerUrl:      t.algo.Account.ExchangeInfo.ExchangeURL,
			ApiSecret:      config.APISecret,
			ApiKey:         config.APIKey,
			AccountID:      "test",
			OutputResponse: false,
		}
		t.algo.Client, err = tradeapi.New(exchangeVars)
		if err != nil {
			logger.Error(err)
		}
	}

	//TODO do we need this order status?
	// t.orderStatus = algo.Client.GetPotentialOrderStatus()

	if t.algo.Account.ExchangeInfo.Options {
		// Build theo engine
		// Assume the first futures market we find is the underlying market
		var underlyingMarket *models.MarketState
		for symbol, marketState := range t.algo.Account.MarketStates {
			if marketState.Info.MarketType == models.Future {
				underlyingMarket = marketState
				logger.Infof("Found underlying market: %v\n", symbol)
				break
			}
		}
		if underlyingMarket == nil {
			log.Fatal("Could not find underlying market for options exchange %v\n", t.algo.Account.ExchangeInfo.Exchange)
		}
		theoEngine := te.NewTheoEngine(underlyingMarket, &t.algo.Timestamp, 60000, 86400000, 0, 0, t.algo.LogLevel)
		t.algo.TheoEngine = &theoEngine
		t.theoEngine = &theoEngine
		logger.Infof("Built new theo engine.\n")
		if isTest {
			theoEngine.CurrentTime = &t.algo.Client.(*tantra.Tantra).CurrentTime
			t.algo.Client.(*tantra.Tantra).SetTheoEngine(&theoEngine)
			// theoEngine.UpdateActiveContracts()
			// theoEngine.ApplyVolSurface()
		}
		logger.Infof("Built theo engine.\n")
	} else {
		logger.Infof("Not building theo engine (no options)\n")
	}

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := t.algo.Client.GetBalances()
	t.updateAlgoBalances(t.algo, balances)

	positions, _ := t.algo.Client.GetPositions(t.algo.Account.BaseAsset.Symbol)
	t.updatePositions(t.algo, positions)

	orders, err := t.algo.Client.GetOpenOrders(iex.OpenOrderF{Currency: t.algo.Account.BaseAsset.Symbol})
	if err != nil {
		logger.Errorf("Error getting open orders: %v\n", err)
	} else {
		logger.Infof("Got %v orders.\n", len(orders))

	}

	// SUBSCRIBE TO WEBSOCKETS
	// channels to subscribe to (only futures and spot for now)
	var subscribeInfos []iex.WSSubscribeInfo
	for marketSymbol, marketState := range t.algo.Account.MarketStates {
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
	err = t.algo.Client.StartWS(&iex.WsConfig{
		Host:      t.algo.Account.ExchangeInfo.WSStream,
		Streams:   subscribeInfos,
		Channels:  channels,
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
			t.updatePositions(t.algo, positions)
			channels.PositionChan <- positions
		case trades := <-channels.TradeBinChan:
			// Update active contracts if we are trading options
			if t.theoEngine != nil {
				t.UpdateActiveContracts()
				t.UpdateMidMarketPrices()
				t.theoEngine.ScanOptions(true, true)
			} else {
				logger.Infof("Cannot update active contracts, theo engine is nil\n")
			}
			// Update your local bars
			for _, trade := range trades {
				t.InsertNewCandle(trade)
				marketState, _ := t.algo.Account.MarketStates[trade.Symbol]
				// t.updateBars(t.algo, trade)
				// now fetch the bars
				// bars := database.GetData(trade.Symbol, t.algo.Account.ExchangeInfo.Exchange, t.algo.RebalanceInterval, t.algo.DataLength+100)
				// Did we get enough data to run this? If we didn't then throw fatal error to notify system
				if t.algo.DataLength < len(marketState.OHLCV.Timestamp) {
					t.updateState(t.algo, trade.Symbol, setupData)
					rebalance(t.algo)
				} else {
					log.Fatalln("Not enough trade data. (local data length", len(marketState.OHLCV.Timestamp), "data length wanted by algo", t.algo.DataLength, ")")
				}
			}
			for _, marketState := range t.algo.Account.MarketStates {
				logState(t.algo, marketState)
				if !isTest {
					t.logLiveState(marketState)
					t.runTest(t.algo, setupData, rebalance)
					t.checkWalletHistory(t.algo, settingsFileName)
				} else {
					// TODO full sync logic?
					// if t.algo.Timestamp == t.algo.Client.(*tantra.Tantra).GetLastTimestamp().UTC() {
					// 	channels.TradeBinChan = nil
					// }
					channels.TradeBinChan <- trades
				}
			}
		case newOrders := <-channels.OrderChan:
			// TODO look at the response for a market order, does it send 2 orders filled and placed or just filled
			t.updateOrders(t.algo, newOrders, true)
			// TODO callback to order function
			channels.OrderChan <- newOrders
		case update := <-channels.WalletChan:
			t.updateAlgoBalances(t.algo, update.Balance)
			channels.WalletChan <- update
		}
		if channels.TradeBinChan == nil {
			logger.Errorf("Trade bin channel is nil, breaking...\n")
			break
		}
	}
	logger.Infof("Reached end of connect.\n")
}

func (t *TradingEngine) checkWalletHistory(algo *models.Algo, settingsFileName string) {
	timeSinceLastSync := database.GetBars()[algo.Index].Timestamp - t.lastWalletSync
	if timeSinceLastSync > (60 * 60 * 60) {
		logger.Info("It has been", timeSinceLastSync, "seconds since the last wallet history download, fetching latest deposits and withdrawals.")
		t.lastWalletSync = database.GetBars()[algo.Index].Timestamp
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
func (t *TradingEngine) updateOrders(algo *models.Algo, orders []iex.Order, isUpdate bool) {
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
func (t *TradingEngine) runTest(algo *models.Algo, setupData func(*models.Algo), rebalance func(*models.Algo)) {
	if t.lastTest != database.GetBars()[algo.Index].Timestamp {
		t.lastTest = database.GetBars()[algo.Index].Timestamp
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

func (t *TradingEngine) updatePositions(algo *models.Algo, positions []iex.WsPosition) {
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
				if t.firstPositionUpdate {
					marketState.ShouldHaveQuantity = marketState.Position
				}
				logState(algo, marketState)
			}
		}
	}
	t.firstPositionUpdate = false
}

func (t *TradingEngine) updateAlgoBalances(algo *models.Algo, balances []iex.WSBalance) {
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

func (t *TradingEngine) updateBars(algo *models.Algo, trade iex.TradeBin) {
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
	logger.Info("Time Elapsed", t.startTime.Sub(time.Now()), "Index", algo.Index)
}

func (t *TradingEngine) updateState(algo *models.Algo, symbol string, setupData func(*models.Algo)) {
	marketState, ok := algo.Account.MarketStates[symbol]
	if !ok {
		logger.Errorf("Cannot update state for %v (could not find market state).\n", symbol)
		return
	}
	setupData(algo)
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
	algo.Timestamp = time.Unix(marketState.Bar.Timestamp/1000, 0).UTC()
	marketState.LastPrice = marketState.Bar.Close
	// logger.Info("algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if t.firstTrade {
		logState(algo, marketState)
		t.firstTrade = false
	}
}

func (t *TradingEngine) UpdateActiveContracts() {
	logger.Infof("Updating active contracts at %v\n", t.algo.Timestamp)
	updateTime := t.lastContractUpdate + t.contractUpdatePeriod
	currentTimestamp := utils.TimeToTimestamp(t.algo.Timestamp)
	if updateTime > currentTimestamp {
		logger.Infof("Skipping contract update. (next update at %v, current time %v)\n", updateTime, currentTimestamp)
		return
	}
	activeOptions := t.GetActiveContracts()
	logger.Infof("Found %v new active options.\n", len(activeOptions))
	for symbol, marketState := range activeOptions {
		// TODO is this check necessary? may already happen in GetActiveContracts()
		_, ok := t.theoEngine.Options[symbol]
		if !ok {
			t.theoEngine.Options[symbol] = &marketState
		}
	}
	t.theoEngine.UpdateOptionIndexes()
	t.lastContractUpdate = currentTimestamp
}

func (t *TradingEngine) GetActiveContracts() map[string]models.MarketState {
	logger.Infof("Generating active contracts at %v\n", t.algo.Timestamp)
	liveContracts := make(map[string]models.MarketState)
	var optionTheo models.OptionTheo
	var marketInfo models.MarketInfo
	var marketState models.MarketState
	var optionType models.OptionType
	if t.algo.Account.ExchangeInfo.Exchange == "deribit" {
		markets, err := t.algo.Client.GetMarkets(t.algo.Account.BaseAsset.Symbol, true, "option")
		logger.Infof("Got %v markets.\n", len(markets))
		if err == nil {
			for _, market := range markets {
				_, ok := t.theoEngine.Options[market.Symbol]
				if !ok {
					optionTheo = models.NewOptionTheo(
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
					marketState = models.MarketState{
						Symbol:         marketInfo.Symbol,
						Info:           marketInfo,
						Orders:         make(map[string]*iex.Order),
						OptionTheo:     &optionTheo,
						MidMarketPrice: market.MidMarketPrice,
					}
					liveContracts[marketState.Symbol] = marketState
					logger.Debugf("Set mid market price for %v: %v\n", market.Symbol, market.MidMarketPrice)
				}
			}
		} else {
			logger.Errorf("Error getting markets: %v\n", err)
		}
	} else {
		logger.Errorf("GetOptionsContracts() not implemented for exchange %v\n", t.algo.Account.ExchangeInfo.Exchange)
	}
	logger.Infof("Found %v live contracts.\n", len(liveContracts))
	return liveContracts
}

func (t *TradingEngine) UpdateMidMarketPrices() {
	if t.algo.Account.ExchangeInfo.Options {
		logger.Debugf("Updating mid markets at %v with currency %v\n", t.algo.Timestamp, t.algo.Account.BaseAsset.Symbol)
		marketPrices, err := t.algo.Client.GetMarketPricesByCurrency(t.algo.Account.BaseAsset.Symbol)
		if err != nil {
			logger.Errorf("Error getting market prices for %v: %v\n", t.algo.Account.BaseAsset.Symbol, err)
			return
		}
		for symbol, price := range marketPrices {
			option, ok := t.theoEngine.Options[symbol]
			if ok {
				option.MidMarketPrice = price
			}
		}
	} else {
		logger.Infof("Exchange does not support options, no need to update mid market prices.\n")
	}
}

func (t *TradingEngine) logTrade(trade iex.Order) {
	stateType := "live"
	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": t.algo.Name, "commit_hash": t.commitHash, "state_type": stateType, "side": strings.ToLower(trade.Side)}

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
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": t.algo.Name, "commit_hash": t.commitHash, "state_type": stateType, "side": strings.ToLower(trade.Side)}

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

//Log the state of the algo to influx db
func (t *TradingEngine) logLiveState(marketState *models.MarketState, test ...bool) {
	stateType := "live"
	if test != nil {
		stateType = "test"
	}

	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": t.algo.Name, "commit_hash": t.commitHash, "state_type": stateType}

	fields := structs.Map(marketState)

	//TODO: shouldn't have to manually delete Options param here
	_, ok := fields["Options"]
	if ok {
		delete(fields, "Options")
	}

	fields["Price"] = marketState.Bar.Close
	fields["Balance"] = t.algo.Account.BaseAsset.Quantity
	fields["Quantity"] = marketState.Position

	pt, err := client.NewPoint(
		"market",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	fields = t.algo.Params[marketState.Symbol]

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
	for symbol, option := range t.algo.Account.MarketStates {
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

	for _, order := range marketState.Orders {
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
	}

	if t.algo.State != nil {
		pt, err := client.NewPoint(
			"state",
			tags,
			t.algo.State,
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

func getPositionAbsLoss(algo *models.Algo, marketState *models.MarketState) float64 {
	positionLoss := 0.0
	if marketState.Position < 0 {
		positionLoss = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
	} else {
		positionLoss = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
	}
	return positionLoss
}

func getPositionAbsProfit(algo *models.Algo, marketState *models.MarketState) float64 {
	positionProfit := 0.0
	if marketState.Position > 0 {
		positionProfit = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
	} else {
		positionProfit = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
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

func canBuy(algo *models.Algo, marketState *models.MarketState) float64 {
	if marketState.CanBuyBasedOnMax {
		return (algo.Account.BaseAsset.Quantity * marketState.Bar.Open) * marketState.MaxLeverage
	} else {
		return (algo.Account.BaseAsset.Quantity * marketState.Bar.Open) * marketState.LeverageTarget
	}
}

//Log the state of the algo and update variables like leverage
func logState(algo *models.Algo, marketState *models.MarketState, timestamp ...time.Time) (state models.History) {
	// algo.History.Timestamp = append(algo.History.Timestamp, timestamp)
	var balance float64
	if marketState.Info.MarketType == models.Future {
		balance = algo.Account.BaseAsset.Quantity
		marketState.Leverage = math.Abs(marketState.Position) / (marketState.Bar.Close * balance)
	} else {
		if marketState.AverageCost == 0 {
			marketState.AverageCost = marketState.Bar.Close
		}
		balance = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		marketState.Leverage = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) / balance
		// log.Println("BaseAsset Quantity", algo.Account.BaseAsset.Quantity, "QuoteAsset Value", marketState.Position/marketState.Bar)
		// log.Println("BaseAsset Value", algo.Account.BaseAsset.Quantity*models.MarketState.Bar, "QuoteAsset Quantity", marketState.Position)
		// log.Println("Leverage", marketState.Leverage)
	}

	// fmt.Println(algo.Timestamp, "Funds", algo.Account.BaseAsset.Quantity, "Quantity", marketState.Position)
	// fmt.Println(algo.Timestamp, algo.Account.BaseAsset.Quantity, algo.CurrentProfit(marketState.Bar))
	marketState.Profit = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Close) * marketState.Leverage))
	// fmt.Println(algo.Timestamp, marketState.Profit)

	if timestamp != nil {
		algo.Timestamp = timestamp[0]
		state = models.History{
			Timestamp:   algo.Timestamp.String(),
			Balance:     balance,
			Quantity:    marketState.Position,
			AverageCost: marketState.AverageCost,
			Leverage:    marketState.Leverage,
			Profit:      marketState.Profit,
			Weight:      int(marketState.Weight),
			MaxLoss:     getPositionAbsLoss(algo, marketState),
			MaxProfit:   getPositionAbsProfit(algo, marketState),
			Price:       marketState.Bar.Close,
		}

		if marketState.Info.MarketType == models.Future {
			if math.IsNaN(marketState.Profit) {
				state.UBalance = balance
			} else {
				state.UBalance = balance + marketState.Profit
			}
		} else {
			state.UBalance = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		}
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Account.BaseAsset.Quantity*marketState.Bar.Close+(marketState.Position), 0.0, algo.Account.BaseAsset.Quantity, marketState.Position, marketState.Bar.Close, marketState.AverageCost))
	}
	return
}

func getOrderSize(algo *models.Algo, marketState *models.MarketState, currentPrice float64, live ...bool) (orderSize float64, side float64) {
	currentWeight := math.Copysign(1, marketState.Position)
	if marketState.Position == 0 {
		currentWeight = float64(marketState.Weight)
	}
	adding := currentWeight == float64(marketState.Weight)
	// fmt.Printf("CURRENT WEIGHT %v, adding %v, leverage target %v, can buy %v, deleverage order size %v\n", currentWeight, adding, marketState.LeverageTarget, canBuy(algo), marketState.DeleverageOrderSize)
	// fmt.Printf("Getting order size with quote asset quantity: %v\n", marketState.Position)

	// Change order sizes for live to ensure similar boolen checks
	exitOrderSize := marketState.ExitOrderSize
	entryOrderSize := marketState.EntryOrderSize
	deleverageOrderSize := marketState.DeleverageOrderSize

	if live != nil && live[0] {
		if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
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
	} else if math.Abs(marketState.Position) > canBuy(algo, marketState)*(1+deleverageOrderSize) && adding {
		orderSize = marketState.DeleverageOrderSize
		side = float64(currentWeight * -1)
	} else if marketState.Weight == 0 && marketState.Leverage > 0 {
		orderSize = getExitOrderSize(marketState, exitOrderSize > marketState.Leverage)
		//side = Opposite of the quantity
		side = -math.Copysign(1, marketState.Position)
	} else if canBuy(algo, marketState) > math.Abs(marketState.Position) {
		// If I can buy more, place order to fill diff of canBuy and current quantity
		orderSize = utils.CalculateDifference(canBuy(algo, marketState), math.Abs(marketState.Position))
		side = float64(marketState.Weight)
	}
	return
}

func getFillPrice(algo *models.Algo, marketState *models.MarketState) float64 {
	var fillPrice float64
	if algo.FillType == exchanges.FillType().Worst {
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
	} else if algo.FillType == exchanges.FillType().Close {
		fillPrice = marketState.Bar.Close
	} else if algo.FillType == exchanges.FillType().Open {
		fillPrice = marketState.Bar.Open
	} else if algo.FillType == exchanges.FillType().MeanOC {
		fillPrice = (marketState.Bar.Open + marketState.Bar.Close) / 2
	} else if algo.FillType == exchanges.FillType().MeanHL {
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
