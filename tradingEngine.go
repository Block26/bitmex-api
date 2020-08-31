// The yantra package contains all base layer components of the Tantra Labs algorithmic trading platform.
// The primary compenent is a trading engine used for interfacing with exchanges and managing requests in both
// backtesting and live environments.
package yantra

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	firebase "firebase.google.com/go"
	"github.com/gocarina/gocsv"
	"github.com/jinzhu/copier"
	"github.com/tantralabs/logger"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/database"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/tantra"
	"github.com/tantralabs/yantra/utils"
	"google.golang.org/api/option"

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
)

var currentRunUUID time.Time
var index int = 0

// var lastTimestamp map[string]int
var fillVolume float64 = 0.0
var realBalance float64 = 0.0

const additionalLiveData int = 3000
const checkWalletHistoryInterval int = 60
const liveTestInterval int = 15

// The trading engine is responsible for managing communication between algos and other modules and the exchange.
type TradingEngine struct {
	Algo                 *models.Algo
	ReuseData            bool
	firstTrade           bool
	firstPositionUpdate  bool
	isTest               bool
	PaperTrade           bool
	commitHash           string
	lastTest             int64
	lastWalletSync       int64
	startTime            time.Time
	endTime              time.Time
	theoEngine           *te.TheoEngine
	lastContractUpdate   int
	contractUpdatePeriod int
	BarData              map[string][]*models.Bar
	preloadBarData       bool
	csvBarDataFile       string
	shouldExportResult   bool
	jsonResultFile       string
}

// Construct a new trading engine given an algo and other configurations.
func NewTradingEngine(algo *models.Algo, contractUpdatePeriod int, reuseData ...bool) TradingEngine {
	currentRunUUID = time.Now()
	//TODO: should theo engine and other vars be initialized here?
	if reuseData == nil {
		reuseData = make([]bool, 1)
		reuseData[0] = false
	}

	t := TradingEngine{
		Algo:                 algo,
		firstTrade:           true,
		firstPositionUpdate:  true,
		commitHash:           time.Now().String(),
		lastTest:             0,
		lastWalletSync:       0,
		startTime:            time.Now(),
		ReuseData:            reuseData[0],
		theoEngine:           nil,
		lastContractUpdate:   0,
		contractUpdatePeriod: contractUpdatePeriod,
	}
	t.checkForPreload()
	return t
}

func (t *TradingEngine) checkForPreload() bool {
	for _, arg := range os.Args[1:] {
		if strings.Contains(arg, "data=") {
			// os.Args[1] = "data=...."
			t.preloadBarData = true
			t.shouldExportResult = true
			t.csvBarDataFile = arg[5:]
		} else if strings.Contains(arg, "export=") {
			// os.Args[2] = "export=...."
			t.shouldExportResult = true
			t.jsonResultFile = arg[7:]
		} else if strings.Contains(arg, "log-backtest") {
			t.Algo.LogBacktest = true
		} else if strings.Contains(arg, "log-cloud-backtest") {
			t.Algo.LogCloudBacktest = true
		} else if strings.Contains(arg, "log-csv-backtest") {
			t.Algo.LogCSVBacktest = true
		} else if strings.Contains(arg, "paper") {
			t.PaperTrade = true

		}
	}
	return t.preloadBarData
}

// Run a backtest given a start and end time.
// Provide a rebalance function to be called at every data interval and performs trading logic.
// Optionally, provide a setup data to be called before rebalance to precompute relevant data and metrics for the algo.
// This is the trading engine's entry point for a new backtest.
func (t *TradingEngine) RunTest(start time.Time, end time.Time, live ...bool) {
	t.SetupTest(start, end, live...)
	t.Connect("", false, true)
	if t.shouldExportResult {
		t.ExportResult()
	}
}

func (t *TradingEngine) SetupTest(start time.Time, end time.Time, live ...bool) {
	t.isTest = true
	isLive := false
	if live != nil {
		isLive = true
	}

	if t.PaperTrade {
		start = time.Now().Add(-time.Minute * 60 * 34 * 60)
		end = time.Now()
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
		if !t.preloadBarData {
			t.BarData = t.LoadBarData(t.Algo, start, end)
		} else {
			t.BarData, start, end = t.LoadBarDataFromCSV(t.Algo, t.csvBarDataFile)
			t.ReuseData = true
		}
	}
	for symbol, data := range t.BarData {
		logger.Infof("Loaded %v instances of bar data for %v with start %v and end %v.\n", len(data), symbol, start, end)
	}

	t.SetAlgoCandleData(t.BarData)
	mockExchange.SetCandleData(t.BarData)
	mockExchange.SetCurrentTime(start)
	t.Algo.Client = mockExchange
	t.Algo.Timestamp = start
	t.endTime = end
	t.Algo.SetupData(t.Algo)
}

func (t *TradingEngine) LogToFirebase() {
	ctx := context.Background()

	conf := &firebase.Config{
		DatabaseURL: "https://live-algos.firebaseio.com",
	}

	file := utils.DownloadFirebaseCreds()
	opt := option.WithCredentialsFile(file.Name())

	// Initialize the app with a service account, granting admin privileges
	app, err := firebase.NewApp(ctx, conf, opt)

	if err != nil {
		fmt.Println("error initializing app:", err)
	}

	client, err := app.Database(ctx)
	if err != nil {
		fmt.Println("Error connecting to db:", err)
	}

	// get the name of the algo repo by splitting on / and getting what is left
	algoRepo := strings.Split(t.Algo.Config.Algo, "/")
	algoRepoName := algoRepo[len(algoRepo)-1]
	// then split at . to remove .git
	algoRepo = strings.Split(algoRepoName, ".")
	algoRepoName = algoRepo[0]

	path := "live/" + algoRepoName + "-" + t.Algo.Config.Branch
	ref := client.NewRef(path)

	// TODO this will overwrite and only put 1 state
	for _, ms := range t.Algo.Account.MarketStates {
		leverage := ms.Leverage
		side := math.Copysign(1, ms.Position)
		leverage = leverage * side

		status := models.AlgoStatus{
			Leverage:           leverage,
			ShouldHaveLeverage: ms.ShouldHaveLeverage,
		}

		err = ref.Set(ctx, status)
	}

	if err != nil {
		fmt.Println("Error setting value:", err)
	}
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
	if t.BarData == nil || !t.ReuseData {
		t.BarData = make(map[string][]*models.Bar)
		for symbol, marketState := range algo.Account.MarketStates {
			logger.Infof("Getting data with symbol %v, decisioninterval %v, datalength %v\n", symbol, algo.RebalanceInterval, algo.DataLength+1)
			// TODO handle extra bars to account for dataLength here
			// t.BarData[symbol] = database.GetData(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, algo.DataLength+100)
			t.BarData[symbol] = database.GetCandlesByTimeWithBuffer(symbol, algo.Account.ExchangeInfo.Exchange, algo.RebalanceInterval, start, end, algo.DataLength)
			marketState.Bar = *t.BarData[symbol][len(t.BarData[symbol])-1]
			marketState.LastPrice = marketState.Bar.Close
			logger.Infof("Initialized bar for %v: %v\n", symbol, marketState.Bar)
		}
		return t.BarData
	}
	return t.BarData
}

// LoadBarDataFromCSV loads bar data from a csv and stores in t.BarData
func (t *TradingEngine) LoadBarDataFromCSV(algo *models.Algo, filePath string) (map[string][]*models.Bar, time.Time, time.Time) {
	if t.BarData == nil || !t.ReuseData {
		t.BarData = make(map[string][]*models.Bar)
		dataFile, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			log.Fatal("Failed to open file when loading barData from CSV:" + err.Error())
		}
		var bars []*models.Bar
		if err := gocsv.UnmarshalFile(dataFile, &bars); err != nil { // Load bars from file
			log.Fatal("Failed to unmarshal data when loading barData from CSV:" + err.Error())
		}
		if err := dataFile.Close(); err != nil {
			log.Fatal("Failed to close file when loading barData from CSV:" + err.Error())
		}
		if len(bars) < 2 {
			log.Fatal("Failed to load barData from CSV: len of barData from csv file must be greater than 1 bar")
		}
		start := utils.TimestampToTime(int(bars[0].Timestamp))
		end := utils.TimestampToTime(int(bars[len(bars)-1].Timestamp))

		for symbol, marketState := range algo.Account.MarketStates {
			logger.Infof("Preloading data for symbol %v from csv file %v\n", symbol, filePath)
			// TODO handle multiple csv files for diff market states ...
			t.BarData[symbol] = bars
			marketState.Bar = *t.BarData[symbol][len(t.BarData[symbol])-1]
			marketState.LastPrice = marketState.Bar.Close
			logger.Infof("Initialized bar for %v: %v\n", symbol, marketState.Bar)
		}
		return t.BarData, start, end
	}
	return t.BarData, time.Time{}, time.Time{}
}

// Connect is called to connect to an exchange's WS api and begin trading.
// The current implementation will execute rebalance every 1 minute regardless of models.Algo.RebalanceInterval
// This is intentional, look at models.Algo.AutoOrderPlacement to understand this paradigm.
func (t *TradingEngine) Connect(settingsFileName string, secret bool, test ...bool) {
	startTime := time.Now()
	utils.LoadENV(secret)
	if t.PaperTrade && !secret {
		utils.LoadENV(true)
	}
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
	var lastBar *models.Bar
	marketStatehistory := make([]models.History, 0)
	signalStateHistory := make([]map[string]interface{}, 0)
	// lastTimestamp := make(map[string]int, 0)

	if !t.isTest {
		config = utils.LoadSecret(settingsFileName, secret)
		logger.Info("Loaded config for", t.Algo.Account.ExchangeInfo.Exchange, "secret", settingsFileName, "key", config.APIKey)
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
		t.BarData = make(map[string][]*models.Bar)
		for symbol, ms := range t.Algo.Account.MarketStates {
			t.BarData[symbol] = database.GetLatestMinuteData(t.Algo.Client, symbol, ms.Info.Exchange, t.Algo.DataLength+additionalLiveData)
		}
		t.SetAlgoCandleData(t.BarData)
		if err != nil {
			logger.Error(err)
		}
	} else if t.PaperTrade {
		for symbol := range t.Algo.Account.MarketStates {
			lastBar = t.BarData[symbol][len(t.BarData[symbol])-1]
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
	t.UpdateAlgoBalances(t.Algo, balances)

	positions, _ := t.Algo.Client.GetPositions(t.Algo.Account.BaseAsset.Symbol)
	t.UpdatePositions(t.Algo, positions)

	orders, err := t.Algo.Client.GetOpenOrders(iex.OpenOrderF{Currency: t.Algo.Account.BaseAsset.Symbol})
	if err != nil {
		logger.Errorf("Error getting open orders: %v\n", err)
	} else {
		logger.Infof("Got %v orders.\n", len(orders))
		for _, o := range orders {
			t.Algo.Client.CancelOrder(iex.CancelOrderF{Uuid: o.OrderID, Market: o.Market})
			if err != nil {
				fmt.Println("there was an error canceling the order", err.Error())
			} else {
				fmt.Println("canceled order:", o.OrderID)
			}
		}
	}

	// SUBSCRIBE TO WEBSOCKETS
	// channels to subscribe to (only futures and spot for now)
	var subscribeInfos []iex.WSSubscribeInfo
	subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_WALLET})
	for symbol, marketState := range t.Algo.Account.MarketStates {
		if marketState.Info.MarketType == models.Future || marketState.Info.MarketType == models.Spot {
			//Ordering is important, get wallet and position first then market info
			logger.Infof("Subscribing to %v channels.\n", symbol)
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_ORDER, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_POSITION, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_TRADE_BIN_1_MIN, Symbol: symbol, Market: iex.WSMarketType{Contract: iex.WS_SWAP}})
		}
	}

	t.Algo.Client.(*tantra.Tantra).PaperTrade = t.PaperTrade
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
			t.UpdatePositions(t.Algo, positions)
			if t.isTest || t.PaperTrade {
				channels.PositionChanComplete <- nil
			}
		case trades := <-channels.TradeBinChan:
			// fmt.Println("trades[0].Timestamp.Unix() >= lastBar.Timestamp", trades[0].Timestamp.Unix() >= lastBar.Timestamp, trades[0].Timestamp.Unix(), lastBar.Timestamp)
			if t.PaperTrade && t.isTest && trades[0].Timestamp.Unix() >= lastBar.Timestamp/1000 {
				log.Println("Paper trade: last bar", trades[0].Timestamp)
				logger.SetLogLevel(logger.LogLevel().Debug)
				t.isTest = false
				for symbol := range t.Algo.Account.MarketStates {
					t.Algo.Account.MarketStates[symbol].OHLCV.SetIsTest(false)
				}
			}
			// Update your local bars
			for _, trade := range trades {
				t.InsertNewCandle(trade)
				marketState, _ := t.Algo.Account.MarketStates[trade.Symbol]
				t.Algo.Account.BaseAsset.Price = trade.Close

				state := logState(t.Algo, marketState)
				marketStatehistory = append(marketStatehistory, state)
				if !t.isTest {
					t.logLiveState()
				}
				// Did we get enough data to run this? If we didn't then throw fatal error to notify system
				if t.Algo.DataLength < len(marketState.OHLCV.GetMinuteData().Timestamp) {
					t.updateState(t.Algo, trade.Symbol)
					t.Algo.Rebalance(t.Algo)
				} //else {
				// log.Println("Not enough trade data. (local data length", len(marketState.OHLCV.GetMinuteData().Timestamp), "data length wanted by Algo", t.Algo.DataLength, ")")
				// }
			}

			if !t.isTest {
				// t.runTest(t.Algo, setupData, rebalance)
				// t.checkWalletHistory(t.Algo, settingsFileName)
			} else {
				if t.Algo.State != nil && t.Algo.LogStateHistory {
					t.Algo.State["timestamp"] = t.Algo.Timestamp.Unix()
					t.Algo.State["price"] = t.Algo.Account.BaseAsset.Price
					// Create the target map
					storedState := make(map[string]interface{})

					// Copy from the original map to the target map
					for key, value := range t.Algo.State {
						storedState[key] = value
					}
					signalStateHistory = append(signalStateHistory, storedState)
				}
			}
			t.aggregateAccountProfit()
			if t.isTest {
				channels.TradeBinChanComplete <- nil
			} else {
				positions, _ := t.Algo.Client.GetPositions(t.Algo.Account.BaseAsset.Symbol)
				t.UpdatePositions(t.Algo, positions)
				t.LogToFirebase()
				index++
				if t.PaperTrade {
					channels.TradeBinChanComplete <- nil
				}
			}

			// log.Println("t.isTest", t.isTest, "t.endTime", t.endTime, "t.Algo.Timestamp", t.Algo.Timestamp, !t.Algo.Timestamp.Before(t.endTime))
			if !t.PaperTrade && !t.Algo.Timestamp.Before(t.endTime.Add(-1*time.Second)) && t.isTest {
				logger.Infof("Algo timestamp %v past end time %v, killing trading engine.\n", t.Algo.Timestamp, t.endTime)
				logStats(t.Algo, marketStatehistory, startTime)
				logStateHistory(t.Algo, signalStateHistory)
				logBacktest(t.Algo)
				return
			}

		case newOrders := <-channels.OrderChan:
			// TODO look at the response for a market order, does it send 2 orders filled and placed or just filled
			t.UpdateOrders(t.Algo, newOrders, true)
			// TODO callback to order function
			// logger.Infof("Order processing took %v ns\n", time.Now().UnixNano()-startTimestamp)
			if t.isTest || t.PaperTrade {
				channels.OrderChanComplete <- nil
			}
		case update := <-channels.WalletChan:
			t.UpdateAlgoBalances(t.Algo, update)
			if t.isTest || t.PaperTrade {
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
func (t *TradingEngine) UpdateOrders(algo *models.Algo, orders []iex.Order, isUpdate bool) {
	// logger.Infof("Processing %v order updates.\n", len(orders))
	if isUpdate {
		// Add to existing order state
		for _, newOrder := range orders {
			logger.Debug("New order status", strings.ToLower(newOrder.OrdStatus))
			marketState, ok := algo.Account.MarketStates[newOrder.Symbol]
			if !ok {
				continue
			}
			if strings.ToLower(newOrder.OrdStatus) == "open" || strings.ToLower(newOrder.OrdStatus) == "new" {
				marketState.Orders[newOrder.OrderID] = newOrder
			} else {
				if strings.ToLower(newOrder.OrdStatus) == "filled" {
					fillVolume += newOrder.Amount
				}
				delete(marketState.Orders, newOrder.OrderID)
			}
		}
	} else {
		// Overwrite all order states
		openOrderMap := make(map[string]map[string]iex.Order)
		for _, order := range orders {
			_, ok := openOrderMap[order.Symbol]
			if !ok {
				openOrderMap[order.Symbol] = make(map[string]iex.Order)
			}
			openOrderMap[order.Symbol][order.OrderID] = order
		}

		for symbol := range algo.Account.MarketStates {
			orderMap, ok := openOrderMap[symbol]
			if ok {
				algo.Account.MarketStates[symbol].Orders = orderMap
			}
		}
	}
}

// Run a new backtest. This private function is meant to be called while the trading engine is running live, as a means
// of making sure that the current state is similar to the expected state (a safety mechanism).
func (t *TradingEngine) runTest(algo *models.Algo) {
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
		testEngine.RunTest(start, end, true)
		testEngine.logLiveState(true)
		//TODO compare the states
	}
}

// Given a set of websocket position updates, update all relevant market states.
func (t *TradingEngine) UpdatePositions(algo *models.Algo, positions []iex.WsPosition) {
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
		// logger.Errorf("Got position update %v for symbol %v, could not find in account market states.\n", position, position.Symbol)
		return
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
	if marketState.Position == 0 {
		marketState.Leverage = 0
	}
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
func (t *TradingEngine) UpdateAlgoBalances(algo *models.Algo, balances []iex.Balance) {
	// fmt.Println("UpdateAlgoBalances", algo.Account.BaseAsset.Symbol)
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
			realBalance = updatedBalance.Available
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
func (t *TradingEngine) updateState(algo *models.Algo, symbol string) {
	marketState, ok := algo.Account.MarketStates[symbol]
	if !ok {
		logger.Errorf("Cannot update state for %v (could not find market state).\n", symbol)
		return
	}
	if !t.isTest {
		algo.SetupData(t.Algo)
	}
	// logger.Info("Algo.Timestamp", algo.Timestamp, "algo.Index", algo.Index, "Close Price", algo.Market.Price.Close)
	if t.firstTrade {
		marketState.Leverage = 0
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
			"state_type":     stateType,
			"algo_name":      t.Algo.Name,
			"yantra_version": t.Algo.Config.YantraVersion,
			"symbol":         symbol,
		}

		fields := map[string]interface{}{}
		fields["state_type"] = stateType
		fields["Price"] = ms.Bar.Close
		fields["Balance"] = t.Algo.Account.BaseAsset.Quantity
		fields["Quantity"] = ms.Position
		fields["AverageCost"] = ms.AverageCost
		fields["FillVolume"] = fillVolume
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

		// LOG orders placed
		for _, o := range ms.Orders {
			orderFields := map[string]interface{}{}
			orderFields["Quantity"] = o.Amount
			orderFields["Price"] = o.Rate
			orderFields["Symbol"] = o.Symbol
			orderFields["Side"] = o.Side
			orderFields["OrderID"] = o.OrderID
			orderFields["ClOrderID"] = o.ClOrdID

			pt, _ = client.NewPoint(
				"order",
				tags,
				orderFields,
				time.Now(),
			)
			bp.AddPoint(pt)
		}

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
	fillVolume = 0
}

func (t *TradingEngine) customLogLiveState() {
	stateType := "live"

	influx := GetInfluxClient()

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	if err != nil {
		fmt.Println("err", err)
	}

	tags := map[string]string{
		"state_type":     stateType,
		"algo_name":      t.Algo.Name,
		"yantra_version": t.Algo.Config.YantraVersion,
		"symbol":         t.Algo.Account.BaseAsset.Symbol,
	}

	fields := map[string]interface{}{}
	fields["state_type"] = stateType
	fields["Balance"] = t.Algo.Account.BaseAsset.Quantity
	fields["FillVolume"] = fillVolume
	fields["RealizedBalance"] = realBalance
	fields["Quantity"] = 0.0

	for symbol, ms := range t.Algo.Account.MarketStates {
		// fmt.Println("logging", symbol, "info")
		if symbol == t.Algo.Account.BaseAsset.Symbol {
			fields["Price"] = ms.Bar.Close
		}
		fields["Price"] = ms.Bar.Close
		fields["Balance"] = t.Algo.Account.BaseAsset.Quantity
		fields["Quantity"] = ms.Position + fields["Quantity"].(float64)
	}

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
	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()

	fillVolume = 0.0
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
		Timestamp:          algo.Timestamp,
		Symbol:             marketState.Symbol,
		Balance:            marketState.Balance,
		Quantity:           marketState.Position,
		AverageCost:        marketState.AverageCost,
		Leverage:           marketState.Leverage,
		ShouldHaveLeverage: marketState.ShouldHaveLeverage,
		Profit:             marketState.Profit,
		Weight:             int(marketState.Weight),
		MaxLoss:            getMaxPositionAbsLoss(algo, marketState),
		MaxProfit:          getMaxPositionAbsProfit(algo, marketState),
		Price:              marketState.Bar.Close,
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
	if algo.LogBacktest {
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

func (t *TradingEngine) CustomConnect(settingsFileName string, secret bool) {
	utils.LoadENV(secret)

	var err error
	var config models.Secret
	// history := make([]models.History, 0)

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
	t.BarData = make(map[string][]*models.Bar)
	for symbol, ms := range t.Algo.Account.MarketStates {
		t.BarData[symbol] = database.GetLatestMinuteDataFromExchange(t.Algo.Client, symbol, ms.Info.Exchange, t.Algo.DataLength+100)
	}
	t.SetAlgoCandleData(t.BarData)
	if err != nil {
		logger.Error(err)
	}

	// SETUP Algo WITH RESTFUL CALLS
	balances, _ := t.Algo.Client.GetBalances()
	t.UpdateAlgoBalances(t.Algo, balances)

	positions, _ := t.Algo.Client.GetPositions(t.Algo.Account.BaseAsset.Symbol)
	t.UpdatePositions(t.Algo, positions)

	// t.Algo.Client.CancelAllOrders()

	orders, err := t.Algo.Client.GetOpenOrders(iex.OpenOrderF{Currency: t.Algo.Account.BaseAsset.Symbol})
	if err != nil {
		logger.Errorf("Error getting open orders: %v\n", err)
	} else {
		logger.Infof("Got %v orders.\n", len(orders))
		// for _, o := range orders {
		// 	t.Algo.Client.CancelOrder(iex.CancelOrderF{Uuid: o.OrderID, Market: o.Market})
		// }
	}
	t.UpdateOrders(t.Algo, orders, false)

	// SUBSCRIBE TO WEBSOCKETS
	// channels to subscribe to (only futures and spot for now)
	var subscribeInfos []iex.WSSubscribeInfo
	subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_WALLET})
	for symbol, marketState := range t.Algo.Account.MarketStates {
		if marketState.Info.MarketType == models.Future || marketState.Info.MarketType == models.Spot {
			//Ordering is important, get wallet and position first then market info
			logger.Infof("Subscribing to %v channels.\n", symbol)
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_ORDER, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_POSITION, Symbol: symbol})
			subscribeInfos = append(subscribeInfos, iex.WSSubscribeInfo{Name: iex.WS_MS_PRICE, Symbol: symbol})
		}
	}

	logger.Infof("Subscribed to %v channels.\n", len(subscribeInfos))

	// Channels for recieving websocket response.
	channels := &iex.WSChannels{
		PositionChan: make(chan []iex.WsPosition, 1),
		TradeBinChan: make(chan []iex.TradeBin, 1),
		WalletChan:   make(chan []iex.Balance, 1),
		OrderChan:    make(chan []iex.Order, 1),
		Stop:         make(chan error, 1),
	}

	// Start the websocket.
	ctx := context.TODO()
	wg := sync.WaitGroup{}
	wg.Add(1)
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
			t.UpdatePositions(t.Algo, positions)
			t.Algo.OnPositionUpdate(t.Algo)
		case trades := <-channels.TradeBinChan:
			// Update your local bars
			// fmt.Println("Trades", trades)
			for _, trade := range trades {
				// t.InsertNewCandle(trade)
				ms, _ := t.Algo.Account.MarketStates[trade.Symbol]
				ms.BestBid = trade.Low
				ms.BestAsk = trade.High
				// Did we get enough data to run this? If we didn't then throw fatal error to notify system
				t.updateState(t.Algo, trade.Symbol)
			}
			// dont update so often cause lol
			if index%(len(t.Algo.Account.MarketStates)*5) == 0 {
				t.Algo.Rebalance(t.Algo)
			}
			// for _, marketState := range t.Algo.Account.MarketStates {
			// state := logState(t.Algo, marketState)
			// history = append(history, state)
			if index%(len(t.Algo.Account.MarketStates)*5*15) == 0 {
				fmt.Println("Log State & Get Open Orders")
				t.customLogLiveState()
				orders, _ := t.Algo.Client.GetOpenOrders(iex.OpenOrderF{Currency: t.Algo.Account.BaseAsset.Symbol})
				t.UpdateOrders(t.Algo, orders, false)
				// fmt.Println("Total open orders", len(orders))
				// for _, ms := range t.Algo.Account.MarketStates {
				// 	fmt.Println("open orders for", ms.Symbol, len(ms.Orders))
				// }
			}

			// }
			// t.checkWalletHistory(t.Algo, settingsFileName)
			t.aggregateAccountProfit()
			// t.LogToFirebase()
			index++
		case newOrders := <-channels.OrderChan:
			// Make sure the orders are coming from the exchange in the right order.
			t.UpdateOrders(t.Algo, newOrders, true)
			t.Algo.OnOrderUpdate(t.Algo, newOrders)
		case update := <-channels.WalletChan:
			t.UpdateAlgoBalances(t.Algo, update)
		case <-channels.Stop:
			break
		}
	}
	logger.Infof("Reached end of connect.\n")
}

// ExportResult exports t.Algo.Result to a JSON file
func (t *TradingEngine) ExportResult() {
	if t.jsonResultFile == "" {
		t.jsonResultFile = "result.json"
	}

	jsonData, err := json.MarshalIndent(t.Algo.Result, "", "  ")
	if err != nil {
		log.Fatal("Failed to unmarshal result for JSON export:" + err.Error())
	}
	// TODO: Fix to make pretty with nested jsonData (i.e. the params)

	if err = ioutil.WriteFile(t.jsonResultFile, jsonData, 0644); err != nil {
		log.Fatal("Failed to write result for JSON export:" + err.Error())
	}
}
