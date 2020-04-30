package tantra

import (
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/tantralabs/logger"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi/global/clients"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/database"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"
)

func NewTest(vars iex.ExchangeConf, account *models.Account, start time.Time, end time.Time, dataLength int) *Tantra {
	logger.Infof("Init new test with start %v and end %v\n", start, end)
	tantra := New(vars, account)
	tantra.index = dataLength
	tantra.start = start
	tantra.end = end
	return tantra
}

// Mock exchange constructor
func New(vars iex.ExchangeConf, account *models.Account) *Tantra {
	client := clients.NewClient(vars)
	return &Tantra{
		client:                client,
		SimulatedExchangeName: vars.Exchange,
		Account:               account,
		orders:                make(map[string]iex.Order),
		ordersToPublish:       make(map[string]iex.Order),
		db:                    database.NewDB(),
		LogBacktest:           true,
	}
}

const InsertBatchSize = 10000

// Tantra represents a mock exchange client
type Tantra struct {
	client *clients.Client
	iex.IExchange
	marketType            string
	channels              *iex.WSChannels
	SimulatedExchangeName string
	MarketInfos           map[string]models.MarketInfo
	Account               *models.Account
	AccountHistory        []models.AccountHistory
	MarketHistory         []map[string]models.MarketHistory
	TradeHistory          []models.Trade
	PreviousMarketStates  map[string]models.MarketHistory
	CurrentTime           time.Time
	orders                map[string]iex.Order
	ordersToPublish       map[string]iex.Order
	index                 int
	candleData            map[string][]iex.TradeBin
	currentCandle         map[string]iex.TradeBin
	sequenceNumber        int
	start                 time.Time
	end                   time.Time
	theoEngine            *te.TheoEngine
	db                    *sqlx.DB
	lastInsertIndex       int
	LogBacktest           bool
}

func (t *Tantra) SetCandleData(data map[string][]*models.Bar) {
	t.candleData = make(map[string][]iex.TradeBin)
	t.currentCandle = make(map[string]iex.TradeBin)
	// Convert from bar model to iex.TradeBin format
	for symbol, barData := range data {
		var candleData []iex.TradeBin
		var bar iex.TradeBin
		for i := range barData {
			bar = iex.TradeBin{
				Timestamp: utils.TimestampToTime(int(barData[i].Timestamp)),
				Symbol:    symbol,
				Open:      barData[i].Open,
				High:      barData[i].High,
				Low:       barData[i].Low,
				Close:     barData[i].Close,
				Volume:    barData[i].Volume,
			}
			candleData = append(candleData, bar)
		}
		t.candleData[symbol] = candleData
		logger.Debugf("Set %v candles for %v.\n", len(candleData), symbol)
	}
	logger.Debugf("Set candle data for %v symbols.\n", len(t.candleData))
}

func (t *Tantra) StartWS(config interface{}) error {
	logger.Infof("Starting mock exchange websockets...\n")
	conf, ok := config.(*iex.WsConfig)
	if !ok {
		return errors.New("Assertion failed: config")
	}

	t.channels = conf.Channels
	logger.Infof("Started exchange order update channel: %v\n", t.channels.OrderChan)
	logger.Infof("Started exchange trade update channel: %v\n", t.channels.TradeBinChan)

	var numIndexes int
	for _, candleData := range t.candleData {
		numIndexes = len(candleData)
		break
	}
	logger.Infof("Number of indexes found: %v (LogBacktest=%v)\n", numIndexes, t.LogBacktest)
	if t.LogBacktest {
		t.insertHistoryToDB(false)
	}
	go func() {
		for index := t.index; index < numIndexes; index++ {
			startTime := time.Now().UnixNano()
			tradeTime := 0
			fillTime := 0
			positionTime := 0
			balanceTime := 0
			extraTime := 0
			extraStart := time.Now().UnixNano()
			// This is the start of the time step, at this point in time some events have not happened yet
			// so we will fill the orders that were placed at the end of the last time step first
			// then we we publish a new trade bin so that the algo connected can make a decision for the
			// next time interval

			// Iterate through all symbols for the respective account
			// TODO should this all happen synchronously, or in parallel?
			logger.Debugf("New index: %v\n", index)
			var tradeUpdates []iex.TradeBin
			extraTime += int(time.Now().UnixNano() - extraStart)
			for symbol, marketState := range t.Account.MarketStates {
				tradeStart := time.Now().UnixNano()
				if marketState.Info.MarketType != models.Option {
					t.updateCandle(index, symbol)
					currentCandle, ok := t.currentCandle[symbol]
					if ok {
						tradeUpdates = append(tradeUpdates, currentCandle)
					}
				}
				tradeTime += int(time.Now().UnixNano() - tradeStart)
			}

			// Fill all orders with the newest candles
			fillStart := time.Now().UnixNano()
			filledSymbols := t.processFills()
			logger.Debugf("Got filled symbols: %v\n", filledSymbols)
			fillTime += int(time.Now().UnixNano() - fillStart)

			// Send position and balance updates if we have any fills
			// TODO should we send balance updates if no fills?
			var lastBalance, lastPosition, lastAverageCost float64
			var currentMarketState *models.MarketState
			var lastMarketState models.MarketHistory
			var currentOk, lastOk bool
			for symbol := range filledSymbols {
				currentMarketState, currentOk = t.Account.MarketStates[symbol]
				lastMarketState, lastOk = t.PreviousMarketStates[symbol]
				if !currentOk || !lastOk {
					logger.Debugf("Error loading market states for %v\n", symbol)
					lastBalance = 0
					lastPosition = 0
					lastAverageCost = 0
				} else {
					lastBalance = lastMarketState.Balance
					lastPosition = lastMarketState.Position
					lastAverageCost = lastMarketState.AverageCost
				}
				// Has the balance changed? Send a balance update. No? Do Nothing
				// fmt.Println(index, lastMarketState.LastPrice, market.LastPrice, *lastMarketState.Balance, market.Balance)
				balanceStart := time.Now().UnixNano()
				if lastBalance != currentMarketState.Balance {
					wallet := iex.WSWallet{
						Balance: []iex.WSBalance{
							{
								Asset:   currentMarketState.Info.BaseSymbol,
								Balance: currentMarketState.Balance,
							},
						},
					}
					t.channels.WalletChan <- &wallet
					<-t.channels.WalletChanComplete
				}
				balanceTime += int(time.Now().UnixNano() - balanceStart)
				// Has the average price or position changed? Yes? Send a Position update. No? Do Nothing
				// fmt.Println(index, lastMarketState.LastPrice, market.LastPrice, lastMarketState.Position, market.Position)
				positionStart := time.Now().UnixNano()
				if lastAverageCost != currentMarketState.AverageCost || lastPosition != currentMarketState.Position {
					logger.Debugf("Building position update [last=%v, current=%v]\n", lastPosition, currentMarketState.Position)
					pos := []iex.WsPosition{
						iex.WsPosition{
							Symbol:       symbol,
							CurrentQty:   currentMarketState.Position,
							AvgCostPrice: currentMarketState.AverageCost,
						},
					}
					t.channels.PositionChan <- pos
					<-t.channels.PositionChanComplete
				}
				positionTime += int(time.Now().UnixNano() - positionStart)
			}
			extraStart = time.Now().UnixNano()
			t.appendToHistory()
			extraTime += int(time.Now().UnixNano() - extraStart)
			// Publish trade updates
			// logger.Infof("Pushing %v candle updates: %v\n", len(tradeUpdates), tradeUpdates)
			t.channels.TradeBinChan <- tradeUpdates
			<-t.channels.TradeBinChanComplete
			if t.LogBacktest {
				t.insertHistoryToDB(false)
			}
			logger.Debugf(`[Exchange] trade time: %v ns
[Exchange] fill time: %v ns
[Exchange] position time: %v ns
[Exchange] balance time: %v ns
[Exchange] extra time: %v ns
[Exchange] timestep took %v ns
[Exchange] cumulative insert time: %v ns`, tradeTime, fillTime, positionTime, balanceTime, extraTime, time.Now().UnixNano()-startTime, insertTime)
		}
		logger.Infof("Fill time: %v ns", fillTime)
		logger.Infof("Insert time: %v ns", insertTime)
		if t.LogBacktest {
			t.insertHistoryToDB(true)
		}
		logger.Infof("Exiting.\n")
		log.Println("Done with test.")
		return
	}()
	logger.Infof("Done with time series iteration.\n")
	return nil
}

func (t *Tantra) SetTheoEngine(theoEngine *te.TheoEngine) {
	t.theoEngine = theoEngine
	logger.Infof("Set theo engine for mock exchange.\n")
	volDataStart := utils.TimeToTimestamp(t.start)
	volDataEnd := utils.TimeToTimestamp(t.end)
	t.theoEngine.InsertVolData(volDataStart, volDataEnd)
	logger.Infof("Inserted vol data with start %v and end %v.\n", volDataStart, volDataEnd)
}

func (t *Tantra) SetCurrentTime(currentTime time.Time) {
	t.CurrentTime = currentTime.UTC()
	logger.Infof("Set current timestamp: %v\n", t.CurrentTime)
}

func (t *Tantra) updateCandle(index int, symbol string) {
	candleData, ok := t.candleData[symbol]
	if !ok {
		return
	}
	//TODO update marketState.Bar here?
	if len(candleData) >= index {
		t.currentCandle[symbol] = candleData[index]
		// logger.Infof("Current candle for %v: %v\n", symbol, t.currentCandle[symbol])
		t.CurrentTime = t.currentCandle[symbol].Timestamp.UTC()
		log.Println("[Exchange] advanced to", t.CurrentTime)
		// logger.Infof("Updated exchange current time: %v\n", t.CurrentTime)
	}
}

func (t *Tantra) getFill(order iex.Order) (isFilled bool, fillPrice, fillAmount float64) {
	lastCandle, ok := t.currentCandle[order.Market]
	if !ok {
		// This is probably an option order, for now simply check if market order
		if order.Type == "market" || order.Rate == 0 {
			option := t.Account.MarketStates[order.Market]
			if option.Info.MarketType == models.Option {
				isFilled = true
				fillPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, order.Side, option.Info.Slippage)
				if order.Side == "sell" {
					fillAmount = -order.Amount
				} else {
					fillAmount = order.Amount
				}
				return
			} else {
				logger.Errorf("Found order for non option market without last candle: %v\n", order)
			}
		}
	}
	// If a future/spot order, check if our current candle fills the high (ask) or low (bid) for the order
	// Price (rate) of zero signifies market order for now
	if order.Type == "market" || order.Rate == 0 {
		isFilled = true
		fillPrice = lastCandle.Close // TODO implement multiple market order fill types
		if order.Side == "buy" {
			fillAmount = order.Amount
		} else if order.Side == "sell" {
			fillAmount = -order.Amount
		}
	} else {
		if order.Side == "buy" && lastCandle.Low <= order.Rate {
			isFilled = true
			fillPrice = order.Rate
			fillAmount = order.Amount
		} else if order.Side == "sell" && lastCandle.High >= order.Rate {
			isFilled = true
			fillPrice = order.Rate
			fillAmount = -order.Amount
		}
	}
	return
}

var fillTime = 0
var insertTime = 0

// Find any orders that should be filled, and update local positions/balances. Then, publish these updates to clients
func (t *Tantra) processFills() (filledSymbols map[string]bool) {
	// Only iterate through current open orders
	filledSymbols = make(map[string]bool)
	var isFilled bool
	var fillAmount, fillPrice float64
	for key, order := range t.orders {
		logger.Debugf("Found open order: %v\n", order)
		marketState, ok := t.Account.MarketStates[order.Market]
		if !ok {
			logger.Errorf("Could not find market state for %v\n", order.Market)
		}
		isFilled, fillPrice, fillAmount = t.getFill(order)
		logger.Debugf("Filled: %v\n", isFilled)
		if isFilled {
			logger.Debugf("Processing fill for order: %v\n", order)
			if t.LogBacktest {
				t.processTrade(models.NewTradeFromOrder(order, utils.TimeToTimestamp(t.CurrentTime)))
			}
			t.updateBalance(&marketState.Balance, &marketState.Position, &marketState.AverageCost, fillPrice, fillAmount, marketState)
			t.prepareOrderUpdate(order, t.GetPotentialOrderStatus().Filled)
			delete(t.orders, key)
			logger.Debugf("Deleted order with key: %v\n", key)
		}
		filledSymbols[order.Market] = true
	}
	if len(t.ordersToPublish) > 0 {
		t.publishOrderUpdates()
	}
	return
}

var appendTime = 0

func (t *Tantra) appendToHistory() {
	appendStart := time.Now().UnixNano()
	timestamp := utils.TimeToTimestamp(t.CurrentTime)
	t.AccountHistory = append(t.AccountHistory, models.NewAccountHistory(*t.Account, timestamp))
	marketStates := make(map[string]models.MarketHistory)
	for symbol, market := range t.Account.MarketStates {
		marketStates[symbol] = models.NewMarketHistory(*market, timestamp)
	}
	t.PreviousMarketStates = marketStates
	t.MarketHistory = append(t.MarketHistory, marketStates)
	logger.Debugf("Appended to history [%v records]\n", len(t.AccountHistory))
	appendTime += int(time.Now().UnixNano() - appendStart)
}

func (t *Tantra) processTrade(trade models.Trade) {
	t.TradeHistory = append(t.TradeHistory, trade)
	logger.Debugf("Processed trade: %v\n", trade)
}

func (t *Tantra) insertHistoryToDB(isLast bool) {
	insertStart := time.Now().UnixNano()
	start := t.lastInsertIndex
	logger.Debugf("Inserting acount history with start %v, len history %v\n", start, len(t.AccountHistory))
	var end int
	if isLast {
		// Insert trade history
		logger.Debugf("Inserting trade history [%v records].\n", len(t.TradeHistory))
		database.InsertTradeHistory(t.db, t.TradeHistory)
		end = len(t.AccountHistory)
	} else {
		if len(t.AccountHistory)-start < InsertBatchSize {
			return
		}
		end = start + InsertBatchSize
	}
	if start == end {
		return
	}
	log.Println("[Exchange] Inserting", end-start, "records...")
	accounts := t.AccountHistory[start:end]
	markets := t.MarketHistory[start:end]
	database.InsertAccountHistory(t.db, accounts)
	database.InsertMarketHistory(t.db, markets)
	logger.Debugf("Inserting trade history [%v records].\n", len(t.TradeHistory))
	database.InsertTradeHistory(t.db, t.TradeHistory)
	t.TradeHistory = []models.Trade{}
	logger.Debugf("Inserted history. [%v records]\n", len(accounts))
	t.lastInsertIndex = end
	insertTime += int(time.Now().UnixNano() - insertStart)
	t.flushHistory()
}

func (t *Tantra) flushHistory() {
	start := t.lastInsertIndex
	end := len(t.AccountHistory) - 1
	if end <= start {
		// Full flush
		t.AccountHistory = []models.AccountHistory{}
		t.MarketHistory = []map[string]models.MarketHistory{}
	} else {
		t.AccountHistory = t.AccountHistory[start:end]
		t.MarketHistory = t.MarketHistory[start:end]
	}
	t.lastInsertIndex = 0
	logger.Debugf("Flushed history (last=%v)\n", end)
}

func (t *Tantra) publishOrderUpdates() {
	logger.Debugf("Publishing %v order updates.\n", len(t.ordersToPublish))
	// t.channels.OrderChan <- t.ordersToPublish
	// <-t.channels.OrderChan
	// logger.Infof("OUTPUT ORDER UPDATE: %v\n", <-t.channels.OrderChan)
	for _, order := range t.ordersToPublish {
		t.channels.OrderChan <- []iex.Order{order}
		<-t.channels.OrderChanComplete
	}
	t.ordersToPublish = make(map[string]iex.Order)
}

func (t *Tantra) updateBalance(currentBaseBalance *float64, currentPosition *float64, averageCost *float64, fillPrice float64, fillAmount float64, marketState *models.MarketState) {
	logger.Debugf("Updating balance with current base balance %v, current position %v, avg cost %v, fill price %v, fill amount %v\n",
		*currentBaseBalance, *currentPosition, *averageCost, fillPrice, fillAmount)
	if math.Abs(fillAmount) > 0 {
		// fee := math.Abs(fillAmount/fillPrice) * t.Account.MakerFee
		// logger.Debugf("fillPrice %.2f -> fillAmount %.2f", fillPrice, fillAmount)
		// logger.Debugf("Updating balance with fill cost %v, fill amount %v, qaq %v, baq %v", fillPrice, fillAmount, currentPosition, currentBaseBalance)
		currentCost := (*currentPosition * *averageCost)
		if marketState.Info.MarketType == models.Future {
			totalQuantity := *currentPosition + fillAmount
			newCost := fillPrice * fillAmount
			if (fillAmount >= 0 && *currentPosition >= 0) || (fillAmount <= 0 && *currentPosition <= 0) {
				//Adding to position
				*averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else if ((fillAmount >= 0 && *currentPosition <= 0) || (fillAmount <= 0 && *currentPosition >= 0)) && math.Abs(fillAmount) >= math.Abs(*currentPosition) {
				//Position changed
				var diff float64
				if fillAmount > 0 {
					diff = utils.CalculateDifference(*averageCost, fillPrice)
				} else {
					diff = utils.CalculateDifference(fillPrice, *averageCost)
				}
				// Only use the remaining position that was filled to calculate cost
				portionFillQuantity := math.Abs(*currentPosition)
				logger.Debugf("Updating current base balance w bb %v, portionFillQuantity %v, diff %v, avgcost %v\n", currentBaseBalance, portionFillQuantity, diff, averageCost)
				*currentBaseBalance = *currentBaseBalance + ((portionFillQuantity * diff) / *averageCost)
				*averageCost = fillPrice
			} else {
				//Leaving Position
				var diff float64
				// TODO is this needed?
				// if algo.FillType == "close" {
				// 	fillPrice = t.Account.Price.Open
				// }
				// Use price open to calculate diff for filltype: close or open
				if fillAmount > 0 {
					diff = utils.CalculateDifference(*averageCost, fillPrice)
				} else {
					diff = utils.CalculateDifference(fillPrice, *averageCost)
				}
				logger.Debugf("Updating full fill quantity with baq %v, fillAmount %v, diff %v, avg cost %v\n", currentBaseBalance, fillAmount, diff, averageCost)
				*currentBaseBalance += ((math.Abs(fillAmount) * diff) / *averageCost)
			}
			*currentPosition += fillAmount
			if *currentPosition == 0 {
				*averageCost = 0
			}
		} else if marketState.Info.MarketType == models.Spot {
			fillAmount = fillAmount / fillPrice
			totalQuantity := *currentPosition + fillAmount
			newCost := fillPrice * fillAmount

			if fillAmount >= 0 && *currentPosition >= 0 {
				//Adding to position
				*averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			}

			*currentPosition -= newCost
			*currentBaseBalance += fillAmount
		} else if marketState.Info.MarketType == models.Option {
			totalQuantity := *currentPosition + fillAmount
			newCost := fillPrice * fillAmount
			var denomPrice float64
			if marketState.Info.MarketType == models.Option {
				denomPrice = *marketState.OptionTheo.UnderlyingPrice
			} else {
				denomPrice = t.currentCandle[marketState.Symbol].Close
			}
			logger.Debugf("Got denom price for %v: %v\n", marketState.Symbol, denomPrice)
			if (fillAmount >= 0 && *currentPosition >= 0) || (fillAmount <= 0 && *currentPosition <= 0) {
				//Adding to position
				*averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else if ((fillAmount >= 0 && *currentPosition <= 0) || (fillAmount <= 0 && *currentPosition >= 0)) && math.Abs(fillAmount) >= math.Abs(*currentPosition) {
				//Position changed
				// Only use the remaining position that was filled to calculate cost
				var balanceChange float64
				if marketState.Info.DenominatedInUnderlying {
					balanceChange = *currentPosition * (fillPrice - *averageCost)
				} else {
					balanceChange = *currentPosition * (fillPrice - *averageCost) / denomPrice
				}
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v, denom price %v\n",
					currentBaseBalance, balanceChange, fillPrice, averageCost, denomPrice)
				*currentBaseBalance += +balanceChange
				*averageCost = fillPrice
			} else {
				//Leaving Position
				var balanceChange float64
				if marketState.Info.DenominatedInUnderlying {
					balanceChange = fillAmount * (fillPrice - *averageCost)
				} else {
					balanceChange = fillAmount * (fillPrice - *averageCost) / denomPrice
				}
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v, denom price %v\n",
					currentBaseBalance, balanceChange, fillPrice, averageCost, denomPrice)
				*currentBaseBalance += balanceChange
			}
			*currentPosition += fillAmount
		}
	}
}

func (t *Tantra) getExpirys() map[int]bool {
	expirys := make(map[int]bool)
	for _, option := range t.theoEngine.Options {
		expirys[option.Info.Expiry] = true
	}
	return expirys
}

func (t *Tantra) GetMarkets(currency string, getMidMarket bool, marketType ...string) ([]*iex.Contract, error) {
	logger.Infof("[Tantra] Getting markets for %v\n", currency)
	getOptions := false
	getFutures := false
	if marketType == nil {
		getOptions = true
		getFutures = true
	} else {
		for _, m := range marketType {
			if m == "option" {
				getOptions = true
			} else if m == "future" {
				getFutures = true
			}
		}
	}
	var contracts []*iex.Contract
	var err error
	if getFutures {
		logger.Infof("Getting futures markets at %v\n", t.CurrentTime)
		for symbol, marketState := range t.Account.MarketStates {
			if marketState.Info.MarketType == models.Future {
				contract := iex.Contract{
					BaseCurrency:      marketState.Info.BaseSymbol,
					ContractSize:      marketState.Info.MinimumOrderSize,
					CreationTimestamp: utils.TimeToTimestamp(t.start),
					Expiry:            0,
					Symbol:            symbol,
					IsActive:          true,
					Kind:              "future",
					Leverage:          int(marketState.Info.MaxLeverage),
					MakerCommission:   marketState.Info.MakerFee,
					MinTradeAmount:    marketState.Info.MinimumOrderSize,
					OptionType:        "",
					Currency:          marketState.Info.BaseSymbol,
					SettlementPeriod:  "",
					Strike:            -1,
					TakerCommission:   marketState.Info.TakerFee,
					TickSize:          float64(marketState.Info.QuantityPrecision),
					MidMarketPrice:    marketState.Bar.Close,
				}
				contracts = append(contracts, &contract)
				logger.Infof("Found new futures contract: %v\n", symbol)
			}
		}
	}
	if getOptions {
		logger.Infof("Getting option markets at %v with num weeklys %v, num monthlys %v\n",
			t.CurrentTime, t.Account.ExchangeInfo.NumWeeklyOptions, t.Account.ExchangeInfo.NumMonthlyOptions)
		currentExpirys := t.getExpirys()
		logger.Infof("Current expirys: %v\n", currentExpirys)
		newExpirys := make(map[int]bool)
		currentTime := t.CurrentTime
		for i := 0; i < t.Account.ExchangeInfo.NumWeeklyOptions; i++ {
			expiry := utils.TimeToTimestamp(utils.GetNextFriday(currentTime))
			if !currentExpirys[expiry] {
				newExpirys[expiry] = true
			}
			currentTime = currentTime.Add(time.Hour * 24 * 7)
		}
		currentTime = t.CurrentTime
		for i := 0; i < t.Account.ExchangeInfo.NumMonthlyOptions; i++ {
			expiry := utils.TimeToTimestamp(utils.GetLastFridayOfMonth(currentTime))
			if !currentExpirys[expiry] {
				newExpirys[expiry] = true
			}
			currentTime = currentTime.Add(time.Hour * 24 * 28)
		}
		if len(newExpirys) == 0 {
			logger.Debugf("No new expirys to generate.\n")
			return contracts, err
		}
		logger.Infof("Generated expirys with currentTime %v: %v\n", currentTime, newExpirys)
		if t.Account.ExchangeInfo.OptionStrikeInterval == 0 {
			log.Fatalln("OptionStrikeInterval cannot be 0, does this exchange support options?")
		}
		currentPrice := *t.theoEngine.UnderlyingPrice
		minStrike := utils.RoundToNearest(currentPrice*(1+(t.Account.ExchangeInfo.OptionMinStrikePct/100.)), t.Account.ExchangeInfo.OptionStrikeInterval)
		maxStrike := utils.RoundToNearest(currentPrice*(1+(t.Account.ExchangeInfo.OptionMaxStrikePct/100.)), t.Account.ExchangeInfo.OptionStrikeInterval)
		strikes := utils.Arange(minStrike, maxStrike, t.Account.ExchangeInfo.OptionStrikeInterval)
		logger.Infof("Generated strikes with current price %v, min strike %v, max strike %v, strike interval %v: %v", currentPrice, minStrike, maxStrike, t.Account.ExchangeInfo.OptionStrikeInterval, strikes)
		for expiry := range newExpirys {
			for _, strike := range strikes {
				for _, optionType := range []string{"call", "put"} {
					contract := iex.Contract{
						BaseCurrency:      currency,
						ContractSize:      t.Account.ExchangeInfo.OptionMinimumOrderSize,
						CreationTimestamp: utils.TimeToTimestamp(t.start),
						Expiry:            expiry,
						Symbol:            utils.GetDeribitOptionSymbol(expiry, strike, t.Account.BaseAsset.Symbol, optionType),
						IsActive:          true,
						Kind:              "option",
						Leverage:          1,
						MakerCommission:   t.Account.ExchangeInfo.OptionMakerFee,
						MinTradeAmount:    t.Account.ExchangeInfo.OptionMinimumOrderSize,
						OptionType:        optionType,
						Currency:          currency,
						SettlementPeriod:  "",
						Strike:            strike,
						TakerCommission:   t.Account.ExchangeInfo.OptionTakerFee,
						TickSize:          t.Account.ExchangeInfo.PricePrecision,
						MidMarketPrice:    -1,
					}
					logger.Infof("Generated option contract for %v.\n", contract.Symbol)
					contracts = append(contracts, &contract)
				}
			}
		}
	}
	logger.Infof("Generated contracts (%v).\n", len(contracts))
	t.parseOptionContracts(contracts)
	return contracts, err
}

var theoTime = 0

func (t *Tantra) GetMarketPricesByCurrency(currency string) (priceMap map[string]float64, err error) {
	priceMap = make(map[string]float64)
	if t.theoEngine == nil {
		logger.Errorf("Cannot get market prices without a theo engine.")
		return
	}
	if t.Account.BaseAsset.Symbol != currency {
		logger.Errorf("Cannot get market prices for currency %v with account currency %v.\n", currency, t.Account.BaseAsset.Symbol)
		return
	}
	theoStart := time.Now().UnixNano()
	for _, option := range t.theoEngine.Options {
		option.OptionTheo.CalcTheo(false)
		priceMap[option.Symbol] = option.OptionTheo.Theo
	}
	theoDuration := int(time.Now().UnixNano() - theoStart)
	logger.Infof("Theo duration: %v\n", theoDuration)
	theoTime += theoDuration
	return
}

// This should be done on the client side as well
func (t *Tantra) removeExpiredOptions() {
	currentTimestamp := utils.TimeToTimestamp(t.CurrentTime)
	for symbol, option := range t.theoEngine.Options {
		if option.Info.Expiry >= currentTimestamp {
			delete(t.theoEngine.Options, symbol)
			delete(t.MarketInfos, symbol)
			logger.Infof("Removed expired option %v with expiry %v, currentTimestamp %v\n", option.Symbol, option.Info.Expiry, currentTimestamp)
		}
	}
}

func (t *Tantra) parseOptionContracts(contracts []*iex.Contract) {
	var marketInfo models.MarketInfo
	var optionType models.OptionType
	for _, contract := range contracts {
		if contract.Kind == "option" {
			_, ok := t.MarketInfos[contract.Symbol]
			if !ok {
				if contract.OptionType == "call" {
					optionType = models.Call
				} else if contract.OptionType == "put" {
					optionType = models.Put
				} else {
					logger.Errorf("Bad option type %v, cannot create market info.\n", contract.OptionType)
					continue
				}
				marketInfo = models.NewMarketInfo(contract.Symbol, t.Account.ExchangeInfo)
				marketInfo.Strike = contract.Strike
				marketInfo.Expiry = contract.Expiry
				marketInfo.OptionType = optionType
				logger.Infof("Created new market info for %v.\n", marketInfo.Symbol)
			}
		}
	}
	logger.Infof("Parsed %v contracts.\n", len(contracts))
}

func (t *Tantra) CurrentOptionProfit() float64 {
	currentProfit := 0.
	for _, option := range t.Account.MarketStates {
		if option.Info.MarketType == models.Option {
			currentProfit += option.Profit
		}
	}
	logger.Debugf("Got current option profit: %v\n", currentProfit)
	return currentProfit
}

func (t *Tantra) prepareOrderUpdate(order iex.Order, status string) {
	o := order // make a copy so we can delete it
	o.OrdStatus = status
	logger.Debugf("New order update: %v %v %v at %v (%v)\n", order.Side, order.Amount, order.Market, order.Rate, o.OrdStatus)
	t.ordersToPublish[o.OrderID] = order
}

func (t *Tantra) PlaceOrder(newOrder iex.Order) (uuid string, err error) {
	if newOrder.Amount <= 0 || newOrder.Rate < 0 {
		logger.Errorf("Invalid order: %v\n", newOrder)
		return
	}
	order := newOrder //TODO is copy necessary here?
	//TODO order side to lower case
	order.Side = strings.ToLower(order.Side)
	order.TransactTime = t.CurrentTime
	logger.Debugf("Placing order: %v %v %v at %v\n", order.Side, order.Amount, order.Market, order.Rate)
	uuid = t.CurrentTime.String() + string(t.sequenceNumber)
	t.sequenceNumber += 1
	order.OrderID = uuid
	order.OrdStatus = "Open"
	t.orders[uuid] = order
	t.ordersToPublish[order.OrderID] = order
	return
}

func (t *Tantra) CancelOrder(cancel iex.CancelOrderF) (err error) {
	// It wasn't making a copy so I am just reconstructing the order
	canceledOrder := iex.Order{
		OrderID:   cancel.Uuid,
		Market:    cancel.Market,
		OrdStatus: t.GetPotentialOrderStatus().Cancelled,
	}
	t.ordersToPublish[canceledOrder.OrderID] = canceledOrder
	delete(t.orders, cancel.Uuid)
	_, ok := t.Account.MarketStates[cancel.Market]
	if ok {
		t.Account.MarketStates[cancel.Market].Orders.Delete(cancel.Uuid)
	} else {
		logger.Infof("Cancel for order %v has unknown market %v\n", cancel.Uuid, cancel.Market)
	}
	return
}

func (t *Tantra) GetPotentialOrderStatus() iex.OrderStatus {
	return iex.OrderStatus{
		Filled:    "Filled",
		Rejected:  "Rejected",
		Cancelled: "Canceled",
	}
}

func (t *Tantra) GetLastTimestamp() time.Time {
	return t.CurrentTime
}

func (t *Tantra) GetCandles(symbol string, binSize string, amount int) ([]iex.TradeBin, error) {
	// only fetch data the first time
	candleData, ok := t.candleData[symbol]
	if !ok {
		candleData = []iex.TradeBin{}
		bars := database.GetCandlesByTime(symbol, t.SimulatedExchangeName, binSize, t.start, t.end)
		var tb iex.TradeBin
		for i := range bars {
			tb = iex.TradeBin{
				Open:      bars[i].Open,
				High:      bars[i].High,
				Low:       bars[i].Low,
				Close:     bars[i].Close,
				Volume:    bars[i].Volume,
				Timestamp: time.Unix(bars[i].Timestamp/1000, 0),
			}
			candleData = append(candleData, tb)
		}
		t.candleData[symbol] = candleData
	}
	// after data is fetched for the first time, keep track of how many times this is called,
	// this is now the index keeper for the algo running
	// NOTE: This amount also effects the index so if it is greater than 1 it will skip through the dataset
	t.index += amount

	if t.index > amount {
		return candleData[t.index-amount-1 : t.index], nil
	} else {
		return candleData[t.index-amount : t.index], nil
	}
}

func (t *Tantra) GetBalances() (balance []iex.WSBalance, err error) {
	balance = append(balance, iex.WSBalance{
		Asset:   t.Account.BaseAsset.Symbol,
		Balance: t.Account.BaseAsset.Quantity,
	})
	return
}

func (t *Tantra) GetBalance(currency string) (balance iex.Balance, err error) {
	log.Fatalln("not implemented")
	return
}

func (t *Tantra) GetPositions(currency string) (positions []iex.WsPosition, err error) {
	var pos iex.WsPosition

	for symbol, marketState := range t.Account.MarketStates {
		if marketState.Info.BaseSymbol == currency {
			pos = iex.WsPosition{
				Symbol:       symbol,
				AvgCostPrice: marketState.AverageCost,
				CurrentQty:   marketState.Position,
			}
			positions = append(positions, pos)
		}
	}
	return
}

func (t *Tantra) GetMarketSummary(symbol string, currency string) (market iex.Market, err error) {
	log.Fatalln("not implemented")
	return
}

func (t *Tantra) GetMarketSummaryByCurrency(currency string) (markets []iex.Market, err error) {
	log.Fatalln("not implemented")
	return
}

func (t *Tantra) GetOrderBook(symbol string, currency string) (orderbook iex.OrderBook, err error) {
	log.Fatalln("not implemented")
	return
}

func (t *Tantra) Withdraw(address, currency string, quantity float64, additionInfo ...string) (res iex.WithdrawResponse, err error) {
	log.Fatalln("not implemented")
	return
}

// TODO can keep a non/synced copy of order map for quick queries
func (t *Tantra) GetOpenOrders(vars iex.OpenOrderF) (orders []iex.Order, err error) {
	// TODO this should return currently open orders
	oo := make([]iex.Order, 0)
	for _, order := range t.orders {
		oo = append(oo, order)
	}
	return oo, nil
}

//WalletHistory not available for this exchange
func (t *Tantra) GetWalletHistory(currency string) (res []iex.WalletHistoryItem, err error) {
	err = errors.New(":error: WalletHistory not available for this exchange yet")
	return
}

func (t *Tantra) OpenOrders(f iex.OpenOrderF) (orders iex.OpenOrders, err error) {
	log.Fatalln("not implemented")
	return
}

func (t *Tantra) FormatMarketPair(pair iex.MarketPair) (res string, err error) {
	log.Fatalln("not implemented")
	return
}

func (t *Tantra) PrepareRequest(r *http.Request) (err error) {
	log.Fatalln("not implemented")
	return
}
