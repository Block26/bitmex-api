package tantra

import (
	"errors"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	backtestDB "github.com/tantralabs/backtest-db"
	"github.com/tantralabs/database"
	"github.com/tantralabs/logger"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi/global/clients"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/utils"
	"github.com/tantralabs/yantra/models"
)

func NewTest(vars iex.ExchangeConf, account *models.Account, start time.Time, end time.Time, dataLength int) *Tantra {
	logger.Infof("Init new test with start %v and end %v\n", start, end)
	tantra := New(vars, account)
	tantra.warmUpPeriod = dataLength
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
		AccountHistory:        make([]models.Account, 0),
		orders:                make(map[string]iex.Order),
		newOrders:             make([]iex.Order, 0),
		db:                    backtestDB.NewDB(),
	}
}

// Tantra represents a mock exchange client
type Tantra struct {
	client *clients.Client
	iex.IExchange
	marketType            string
	channels              *iex.WSChannels
	SimulatedExchangeName string
	MarketInfos           map[string]models.MarketInfo
	Account               *models.Account
	AccountHistory        []models.Account
	CurrentTime           time.Time
	orders                map[string]iex.Order
	ordersBySymbol        map[string]map[string]iex.Order
	newOrders             []iex.Order
	index                 int
	warmUpPeriod          int
	candleData            map[string][]iex.TradeBin
	currentCandle         map[string]iex.TradeBin
	start                 time.Time
	end                   time.Time
	theoEngine            *te.TheoEngine
	db                    *sqlx.DB
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
		logger.Infof("Set %v candles for %v.\n", len(candleData), symbol)
	}
	logger.Infof("Set candle data for %v symbols.\n", len(t.candleData))
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
	logger.Infof("Number of indexes found: %v, warm up period: %v\n", numIndexes, t.warmUpPeriod)

	go func() {
		for index := 0; index < numIndexes-t.warmUpPeriod; index++ {
			// This is the start of the time step, at this point in time some events have not happened yet
			// so we will fill the orders that were placed at the end of the last time step first
			// then we we publish a new trade bin so that the algo connected can make a decision for the
			// next time interval

			// Iterate through all symbols for the respective account
			// TODO should this all happen synchronously, or in parallel?
			logger.Infof("New index: %v\n", index)
			var low float64
			var high float64
			var market *models.MarketState
			var ok bool
			var lastAccountState *models.Account
			var lastMarketState *models.MarketState
			var tradeUpdates []iex.TradeBin
			for symbol, marketState := range t.Account.MarketStates {
				market, ok = t.Account.MarketStates[symbol]
				if !ok {
					logger.Errorf("Symbol %v not found in account market states: %v\n", symbol, t.Account.MarketStates)
					continue
				}
				if marketState.Info.MarketType != models.Option {
					low, high = t.updateCandle(index, symbol)
					currentCandle, ok := t.currentCandle[symbol]
					if ok {
						tradeUpdates = append(tradeUpdates, currentCandle)
					}
				} else {
					low = -1
					high = -1
				}
				t.processFills(marketState, low, high)
				lastAccountState = t.getLastAccountHistory()
				lastMarketState, ok = lastAccountState.MarketStates[symbol]
				if !ok {
					logger.Errorf("Could not load last market state for symbol %v with last account state %v\n", symbol, lastAccountState)
					continue
				}
				// Has the balance changed? Send a balance update. No? Do Nothing
				if lastMarketState.Balance != market.Balance {
					wallet := iex.WSWallet{
						Balance: []iex.WSBalance{
							{
								Asset:   symbol,
								Balance: *market.Balance,
							},
						},
					}
					t.channels.WalletChan <- &wallet

					// Wait for channel to complete
					<-t.channels.WalletChan
				}

				// Has the average price or position changed? Yes? Send a Position update. No? Do Nothing
				if lastMarketState.AverageCost != market.AverageCost || lastMarketState.Position != market.Position {
					pos := []iex.WsPosition{
						iex.WsPosition{
							Symbol:       symbol,
							CurrentQty:   market.Position,
							AvgCostPrice: market.AverageCost,
						},
					}
					t.channels.PositionChan <- pos

					// Wait for channel to complete
					<-t.channels.PositionChan
				}
				t.AccountHistory = append(t.AccountHistory, *t.Account)
			}
			// Publish trade updates
			logger.Infof("Pushing %v candle updates: %v\n", len(tradeUpdates), tradeUpdates)
			t.channels.TradeBinChan <- tradeUpdates
			// <-t.channels.TradeBinChan
		}
	}()
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

func (t *Tantra) updateCandle(index int, symbol string) (low, high float64) {
	candleData, ok := t.candleData[symbol]
	if !ok {
		return
	}
	if len(candleData) >= index+t.warmUpPeriod {
		t.currentCandle[symbol] = candleData[index+t.warmUpPeriod]
		logger.Infof("Current candle for %v: %v\n", symbol, t.currentCandle[symbol])
		t.CurrentTime = t.currentCandle[symbol].Timestamp.UTC()
		logger.Infof("Updated exchange current time: %v\n", t.CurrentTime)
		return t.currentCandle[symbol].Low, t.currentCandle[symbol].High
	}
	return -1, -1
}

func (t *Tantra) processFills(marketState *models.MarketState, low, high float64) {
	logger.Infof("Processing fills for %v with low %v and high %v\n", marketState.Symbol, low, high)
	if marketState.Info.MarketType == models.Option {
		t.getOptionFills(marketState)
	} else {
		bidsFilled := t.getFilledBidOrders(marketState.Symbol, low)
		if len(bidsFilled) > 0 {
			fillCost, fillQuantity := t.getCostAverage(bidsFilled)
			t.updateBalance(marketState.Balance, &marketState.Position, &marketState.AverageCost, fillCost, fillQuantity, marketState)
		}
		asksFilled := t.getFilledAskOrders(marketState.Symbol, high)
		if len(asksFilled) > 0 {
			fillCost, fillQuantity := t.getCostAverage(asksFilled)
			t.updateBalance(marketState.Balance, &marketState.Position, &marketState.AverageCost, fillCost, -fillQuantity, marketState)
		}
	}
	if len(t.newOrders) > 0 {
		t.publishOrderUpdates()
	}
}

// Get the last account history, the first time should just return
func (t *Tantra) getLastAccountHistory() *models.Account {
	length := len(t.AccountHistory)
	if length > 0 {
		return &t.AccountHistory[length-1]
	}
	return t.Account
}

func (t *Tantra) publishOrderUpdates() {
	logger.Infof("Publishing %v order updates.\n", len(t.newOrders))
	t.channels.OrderChan <- t.newOrders
	// <-t.channels.OrderChan
	// logger.Infof("OUTPUT ORDER UPDATE: %v\n", <-t.channels.OrderChan)
	// for _, order := range t.newOrders {
	// 	t.channels.OrderChan <- []iex.Order{order}
	// 	// <-t.channels.OrderChan
	// }
	t.newOrders = make([]iex.Order, 0)
}

func (t *Tantra) updateBalance(currentBaseBalance *float64, currentPosition *float64, averageCost *float64, fillPrice float64, fillAmount float64, marketState *models.MarketState) {
	logger.Debugf("Updating balance with current base balance %v, current position %v, avg cost %v, fill price %v, fill amount %v\n",
		*currentBaseBalance, *currentPosition, *averageCost, fillPrice, fillAmount)
	if math.Abs(fillAmount) > 0 {
		// fee := math.Abs(fillAmount/fillPrice) * t.Account.MakerFee
		// logger.Printf("fillPrice %.2f -> fillAmount %.2f", fillPrice, fillAmount)
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
					balanceChange = *currentPosition * (fillPrice - *averageCost) / marketState.Bar.Close
				}
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v", currentBaseBalance, balanceChange, fillPrice, averageCost)
				*currentBaseBalance += +balanceChange
				*averageCost = fillPrice
			} else {
				//Leaving Position
				var balanceChange float64
				if marketState.Info.DenominatedInUnderlying {
					balanceChange = fillAmount * (fillPrice - *averageCost)
				} else {
					balanceChange = fillAmount * (fillPrice - *averageCost) / marketState.Bar.Close
				}
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v\n", currentBaseBalance, balanceChange, fillPrice, averageCost)
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
					TickSize:          marketState.Info.QuantityPrecision,
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
					contracts = append(contracts, &contract)
				}
			}
		}
	}
	logger.Infof("Generated contracts (%v).\n", len(contracts))
	t.parseOptionContracts(contracts)
	return contracts, err
}

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
	for _, option := range t.theoEngine.Options {
		option.OptionTheo.CalcTheo(false)
		priceMap[option.Symbol] = option.OptionTheo.Theo
	}
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

func (t *Tantra) getCostAverage(filledOrders []iex.Order) (averageCost float64, quantity float64) {
	// print(len(prices), len(orders), len(timestamp_arr[0]))
	totalCost := 0.0
	for _, order := range filledOrders {
		totalCost += order.Rate * order.Amount
		quantity += order.Amount
	}
	if quantity > 0 {
		averageCost = totalCost / quantity
		log.Println("getCostAverage", averageCost)
		return
	}

	return 0.0, 0.0
}

func (t *Tantra) getFilledBidOrders(symbol string, price float64) (filledOrders []iex.Order) {
	orders, ok := t.ordersBySymbol[symbol]
	if !ok {
		logger.Debugf("No order map found for symbol %v.\n", symbol)
	} else {
		for _, order := range orders {
			marketState, ok := t.Account.MarketStates[order.Market]
			if !ok {
				logger.Errorf("Order %v market is not in market infos for mock exchange.\n", order)
				continue
			}
			if marketState.Info.MarketType != models.Option && order.Side == "Buy" {
				if order.Type == "Market" {
					o := order // make a copy so we can delete it
					// Update Order Status
					// o.Rate :=  //TODO getMarketFillPrice()
					o.OrdStatus = t.GetPotentialOrderStatus().Filled
					filledOrders = append(filledOrders, o)
					t.prepareOrderUpdate(o, t.GetPotentialOrderStatus().Filled)
				} else if order.Rate > price {
					o := order // make a copy so we can delete it
					// Update Order Status
					o.OrdStatus = t.GetPotentialOrderStatus().Filled
					filledOrders = append(filledOrders, o)
					t.prepareOrderUpdate(o, t.GetPotentialOrderStatus().Filled)
				}
				logger.Infof("Filled trade: %v %v %v at %v\n", order.Side, order.Amount, order.Symbol, order.Rate)
				backtestDB.InsertTrade(t.db, marketState.Info.Symbol, price, order.Amount, "buy", marketState.UnrealizedProfit, marketState.RealizedProfit, marketState.AverageCost, "market", utils.TimeToTimestamp(t.CurrentTime))
			}
		}
	}
	// Delete filled orders from orders open
	for _, order := range filledOrders {
		delete(t.orders, order.OrderID)
	}
	return
}

func (t *Tantra) getFilledAskOrders(symbol string, price float64) (filledOrders []iex.Order) {
	orders, ok := t.ordersBySymbol[symbol]
	if !ok {
		logger.Debugf("No order map found for symbol %v.\n", symbol)
	} else {
		for _, order := range orders {
			marketState, ok := t.Account.MarketStates[order.Market]
			if !ok {
				logger.Errorf("Order %v market is not in market infos for mock exchange.\n", order)
				continue
			}
			if marketState.Info.MarketType != models.Option && order.Side == "sell" {
				if order.Type == "Market" {
					o := order // make a copy so we can delete it
					// Update Order Status
					// o.Rate :=  //TODO getMarketFillPrice()
					o.OrdStatus = t.GetPotentialOrderStatus().Filled
					filledOrders = append(filledOrders, o)
					t.newOrders = append(t.newOrders, o)
				} else if order.Rate < price {
					o := order // make a copy so we can delete it
					// Update Order Status
					o.OrdStatus = t.GetPotentialOrderStatus().Filled
					filledOrders = append(filledOrders, o)
					t.newOrders = append(t.newOrders, o)
				}
				logger.Infof("Filled trade: %v %v %v at %v\n", order.Side, order.Amount, order.Symbol, order.Rate)
				backtestDB.InsertTrade(t.db, marketState.Info.Symbol, price, order.Amount, "sell", marketState.UnrealizedProfit, marketState.RealizedProfit, marketState.AverageCost, "market", utils.TimeToTimestamp(t.CurrentTime))
			}
		}
	}
	// Delete filled orders from orders open
	for _, order := range filledOrders {
		delete(t.orders, order.OrderID)
	}
	return
}

func (t *Tantra) prepareOrderUpdate(order iex.Order, status string) {
	logger.Infof("New order update: %v %v %v at %v (%v)\n", order.Side, order.Amount, order.Market, order.Rate, status)
	o := order // make a copy so we can delete it
	o.OrdStatus = status
	t.newOrders = append(t.newOrders, o)
}

// TODO can this be generalized?
func (t *Tantra) getOptionFills(option *models.MarketState) {
	logger.Infof("Getting option fills for %v\n", option.Symbol)
	var optionPrice float64
	var optionQty float64
	if option.Status == models.Open {
		for orderID, order := range option.Orders {
			optionPrice = order.Rate
			optionQty = order.Amount
			if order.Side == "buy" {
				if optionPrice == 0 {
					// Simulate market order, assume theo is updated
					optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "buy", option.Info.Slippage)
				}
				logger.Debugf("Updating option position for %v: position %v, price %v, qty %v\n", option.Symbol, option.Position, optionPrice, optionQty)
				t.updateBalance(&option.RealizedProfit, &option.Position, &option.AverageCost, optionPrice, optionQty, option)
				logger.Infof("Filled trade: %v %v %v at %v\n", order.Side, order.Amount, order.Symbol, order.Rate)
				backtestDB.InsertTrade(t.db, option.Symbol, optionPrice, optionQty, "buy", option.UnrealizedProfit, option.RealizedProfit, option.AverageCost, "market", utils.TimeToTimestamp(t.CurrentTime))
				logger.Debugf("Updated buy avgcost for option %v: %v with realized profit %v\n", option.Symbol, option.AverageCost, option.RealizedProfit)
				t.prepareOrderUpdate(*order, t.GetPotentialOrderStatus().Filled)
			} else if order.Side == "sell" {
				if optionPrice == 0 {
					// Simulate market order, assume theo is updated
					optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "sell", option.Info.Slippage)
				}
				logger.Debugf("Updating option position for %v: position %v, price %v, qty %v\n", option.Symbol, option.Position, optionPrice, optionQty)
				t.updateBalance(&option.RealizedProfit, &option.Position, &option.AverageCost, optionPrice, optionQty, option)
				logger.Infof("Filled trade: %v %v %v at %v\n", order.Side, order.Amount, order.Symbol, order.Rate)
				backtestDB.InsertTrade(t.db, option.Symbol, optionPrice, optionQty, "buy", option.UnrealizedProfit, option.RealizedProfit, option.AverageCost, "market", utils.TimeToTimestamp(t.CurrentTime))
				logger.Debugf("Updated sell avgcost for option %v: %v with realized profit %v\n", option.Symbol, option.AverageCost, option.RealizedProfit)
				t.prepareOrderUpdate(*order, t.GetPotentialOrderStatus().Filled)
			}
			delete(option.Orders, orderID)
			logger.Debugf("Removed option order with id %v.\n", orderID)
		}
	}
}

func (t *Tantra) PlaceOrder(order iex.Order) (uuid string, err error) {
	//TODO order side to lower case
	order.Side = strings.ToLower(order.Side)
	order.TransactTime = t.CurrentTime
	logger.Infof("Placing order: %v %v %v at %v\n", order.Side, order.Amount, order.Market, order.Rate)
	log.Println("Placing order with price", order.Rate, "amount", order.Amount, "side", order.Side, "symbol", order.Symbol)
	// Create uuid for order
	uuid = t.CurrentTime.String() + string(len(t.orders))
	order.OrderID = uuid
	order.OrdStatus = "Open"
	t.orders[uuid] = order
	t.newOrders = append(t.newOrders, order)
	state, ok := t.Account.MarketStates[order.Market]
	if ok {
		state.Orders[uuid] = &order
		orderMap, ok := t.ordersBySymbol[order.Market]
		if !ok {
			orderSymbolMap := make(map[string]map[string]iex.Order)
			orderSymbolMap[order.Market] = make(map[string]iex.Order)
			orderSymbolMap[order.Market][uuid] = order
			logger.Infof("Built order map by symbol for order: %v\n", order)
		} else {
			orderMap[uuid] = order
		}
	} else {
		logger.Errorf("Got order %v for unknown market: %v (num markets=%v)\n", uuid, order.Market, len(t.Account.MarketStates))
	}
	return
}

func (t *Tantra) CancelOrder(cancel iex.CancelOrderF) (err error) {
	// It wasn't making a copy so I am just reconstructing the order
	canceledOrder := iex.Order{
		OrderID:   cancel.Uuid,
		OrdStatus: t.GetPotentialOrderStatus().Cancelled,
	}
	t.newOrders = append(t.newOrders, canceledOrder)
	delete(t.orders, cancel.Uuid)
	state, ok := t.Account.MarketStates[cancel.Market]
	if ok {
		delete(state.Orders, cancel.Uuid)
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

func (t *Tantra) GetData(symbol string, binSize string, amount int) ([]iex.TradeBin, error) {
	// only fetch data the first time
	candleData, ok := t.candleData[symbol]
	if !ok {
		candleData = []iex.TradeBin{}
		bars := database.GetData(symbol, t.SimulatedExchangeName, binSize, t.start, t.end)
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
	return
}

func (t *Tantra) GetMarketSummaryByCurrency(currency string) (markets []iex.Market, err error) {
	return
}

func (t *Tantra) GetOrderBook(symbol string, currency string) (orderbook iex.OrderBook, err error) {
	return
}

func (t *Tantra) Withdraw(address, currency string, quantity float64, additionInfo ...string) (res iex.WithdrawResponse, err error) {
	return
}

func (t *Tantra) GetOpenOrders(vars iex.OpenOrderF) (orders []iex.Order, err error) {
	// TODO this should return currently open orders
	orders = make([]iex.Order, 0)
	return
}

//WalletHistory not available for this exchange
func (t *Tantra) GetWalletHistory(currency string) (res []iex.WalletHistoryItem, err error) {
	err = errors.New(":error: WalletHistory not available for this exchange yet")
	return
}

func (t *Tantra) OpenOrders(f iex.OpenOrderF) (orders iex.OpenOrders, err error) {
	return
}

func (t *Tantra) FormatMarketPair(pair iex.MarketPair) (res string, err error) {
	return
}

func (t *Tantra) PrepareRequest(r *http.Request) (err error) {
	return
}
