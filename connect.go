package yantra

import (
	"fmt"
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
		lastContractUpdate:   utils.TimeToTimestamp(time.Now().UTC()),
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
	mockExchange.SetCandleData(barData)
	mockExchange.SetCurrentTime(start)
	t.algo.Client = mockExchange
	t.algo.Timestamp = start

	t.Connect("", false, *t.algo, rebalance, setupData, true)
}

func (t *TradingEngine) LoadBarData(algo *models.Algo, start time.Time, end time.Time) map[string][]*models.Bar {
	barData := make(map[string][]*models.Bar)
	for symbol, marketState := range algo.Account.MarketStates {
		logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", symbol, algo.RebalanceInterval, algo.DataLength+1)
		// TODO handle extra bars to account for dataLength here
		// barData[symbol] = database.GetData(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, algo.DataLength+100)
		barData[symbol] = database.GetDataByTime(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, start, end)
		marketState.Bar = *barData[symbol][len(barData[symbol])-1]
		marketState.LastPrice = marketState.Bar.Close
		logger.Infof("Initialized bar for %v: %v\n", symbol, marketState.Bar)
	}
	return barData
}

// Connect is called to connect to an exchange's WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of models.Algo.RebalanceInterval
// This is intentional, look at models.Algo.AutoOrderPlacement to understand this paradigm.
func (t *TradingEngine) Connect(settingsFileName string, secret bool, algo models.Algo, rebalance func(*models.Algo), setupData func(*models.Algo), test ...bool) {
	utils.LoadENV(secret)
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

	//TODO do we need this order status?
	// t.orderStatus = algo.Client.GetPotentialOrderStatus()

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
		theoEngine := te.NewTheoEngine(underlyingMarket, &algo.Timestamp, 60000, 86400000, 0, 0, t.algo.LogLevel)
		algo.TheoEngine = &theoEngine
		if isTest {
			theoEngine.CurrentTime = &t.algo.Client.(*tantra.Tantra).CurrentTime
			t.algo.Client.(*tantra.Tantra).SetTheoEngine(&theoEngine)
			// theoEngine.UpdateActiveContracts()
			// theoEngine.ApplyVolSurface()
		}
		logger.Infof("Built theo engine.\n")
	}

	// SETUP ALGO WITH RESTFUL CALLS
	balances, _ := algo.Client.GetBalances()
	t.updateAlgoBalances(&algo, balances)

	positions, _ := algo.Client.GetPositions(algo.Account.BaseAsset.Symbol)
	t.updatePositions(&algo, positions)

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

	logger.Infof("Subscribing to %v channels.\n", len(subscribeInfos))

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
		msg := fmt.Sprintf("Error starting websockets: %v\n", err)
		log.Fatal(msg)
	}

	// All of these channels send themselves back so that the test can wait for each individual to complete
	for {
		select {
		case positions := <-channels.PositionChan:
			t.updatePositions(&algo, positions)
			channels.PositionChan <- positions
		case trades := <-channels.TradeBinChan:
			// Update active contracts if we are trading options
			if t.theoEngine != nil {
				t.UpdateActiveContracts()
				t.UpdateMidMarketPrices()
				t.theoEngine.ScanOptions(true, true)
			}
			// Update your local bars
			for _, trade := range trades {
				t.updateBars(&algo, trade)
				// now fetch the bars
				bars := database.GetData(trade.Symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, algo.DataLength+100)
				// Did we get enough data to run this? If we didn't then throw fatal error to notify system
				if algo.DataLength < len(bars) {
					t.updateState(&algo, trade.Symbol, bars, setupData)
					rebalance(&algo)
				} else {
					log.Fatalln("Not enough trade data. (local data length", len(bars), "data length wanted by algo", algo.DataLength, ")")
				}
				// setupOrders(&algo, trade[0].Close)
				// placeOrdersOnBook(&algo, localOrders)
			}
			for _, marketState := range algo.Account.MarketStates {
				logState(&algo, marketState)
				if !isTest {
					t.logLiveState(marketState)
					t.runTest(&algo, setupData, rebalance)
					t.checkWalletHistory(&algo, settingsFileName)
				} else {
					if algo.Timestamp == t.algo.Client.(*tantra.Tantra).GetLastTimestamp().UTC() {
						channels.TradeBinChan = nil
					} else {
						channels.TradeBinChan <- trades
					}
				}
			}
		case newOrders := <-channels.OrderChan:
			// TODO look at the response for a market order, does it send 2 orders filled and placed or just filled
			t.updateOrders(&algo, newOrders, true)
			// TODO callback to order function
			channels.OrderChan <- newOrders
		case update := <-channels.WalletChan:
			t.updateAlgoBalances(&algo, update.Balance)
			channels.WalletChan <- update
		}
		if channels.TradeBinChan == nil {
			break
		}
	}
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

func (t *TradingEngine) updateState(algo *models.Algo, symbol string, bars []*models.Bar, setupData func(*models.Algo)) {
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
	if t.firstTrade {
		logState(algo, marketState)
		t.firstTrade = false
	}
}

func (t *TradingEngine) UpdateActiveContracts() {
	logger.Debugf("Updating active contracts at %v\n", t.algo.Timestamp)
	updateTime := t.lastContractUpdate + t.contractUpdatePeriod
	currentTimestamp := utils.TimeToTimestamp(t.algo.Timestamp)
	if updateTime > currentTimestamp {
		logger.Debugf("Skipping contract update. (next update at %v, current time %v)\n", updateTime, currentTimestamp)
		return
	}
	activeOptions := t.GetActiveContracts()
	logger.Livef("Found %v new active options.\n", len(activeOptions))
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
	logger.Debugf("Generating live options at %v\n", t.algo.Timestamp)
	liveContracts := make(map[string]models.MarketState)
	var optionTheo models.OptionTheo
	var marketInfo models.MarketInfo
	var marketState models.MarketState
	var optionType models.OptionType
	if t.algo.Account.ExchangeInfo.Exchange == "deribit" {
		markets, err := t.algo.Client.GetMarkets(t.algo.Account.BaseAsset.Symbol, true, "option")
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

	fields = t.algo.Params

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
