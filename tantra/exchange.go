package tantra

import (
	"errors"
	"log"
	"math"
	"time"

	"github.com/tantralabs/database"
	"github.com/tantralabs/exchanges"
	"github.com/tantralabs/logger"
	"github.com/tantralabs/models"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/tradeapi/global/clients"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/utils"
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
	newOrders             []iex.Order
	index                 int
	warmUpPeriod          int
	candleData            []iex.TradeBin
	start                 time.Time
	end                   time.Time
	theoEngine            *te.TheoEngine
}

func (t *Tantra) StartWS(config interface{}) error {
	conf, ok := config.(*iex.WsConfig)
	if !ok {
		return errors.New("Assertion failed: config")
	}

	t.channels = conf.Channels

	go func() {
		for index := 0; index < len(t.candleData)-t.warmUpPeriod; index++ {
			// This is the start of the time step, at this point in time some events have not happened yet
			// so we will fill the orders that were placed at the end of the last time step first
			// then we we publish a new trade bin so that the algo connected can make a decision for the
			// next time interval
			row := t.candleData[index+t.warmUpPeriod]
			t.CurrentTime = row.Timestamp
			logger.Infof("Updated exchange current time: %v\n", t.CurrentTime)

			//Check if bids filled
			bidsFilled := t.getFilledBidOrders(row.Low)
			fillCost, fillQuantity := t.getCostAverage(bidsFilled)
			t.updateBalance(t.Account.BaseAsset.Quantity, t.Account.QuoteAsset.Quantity, t.Account.AverageCost, fillCost, fillQuantity, t.marketType, true)
			//Check if asks filled
			asksFilled := t.getFilledAskOrders(row.High)
			fillCost, fillQuantity = t.getCostAverage(asksFilled)
			t.updateBalance(t.Account.BaseAsset.Quantity, t.Account.QuoteAsset.Quantity, t.Account.AverageCost, fillCost, -fillQuantity, t.marketType, true)

			t.respondWithOrderChanges()

			lastAccountState := t.getLastAccountHistory()

			// Has the balance changed? Send a balance update. No? Do Nothing
			if lastAccountState.BaseAsset.Quantity != t.Account.BaseAsset.Quantity {
				wallet := iex.WSWallet{
					Balance: []iex.WSBalance{
						{
							Asset:   t.Account.BaseAsset.Symbol,
							Balance: t.Account.BaseAsset.Quantity,
						},
					},
				}
				t.channels.WalletChan <- &wallet

				// Wait for channel to complete
				<-t.channels.WalletChan
			}

			// Has the Cost Average changed, or the Quote Asset Quantity? Yes? Send a Position update. No? Do Nothing
			if lastAccountState.QuoteAsset.Quantity != t.Account.QuoteAsset.Quantity || lastAccountState.AverageCost != t.Account.AverageCost {
				pos := []iex.WsPosition{
					iex.WsPosition{
						Symbol:       t.Account.QuoteAsset.Symbol,
						CurrentQty:   t.Account.QuoteAsset.Quantity,
						AvgCostPrice: t.Account.AverageCost,
					},
				}
				t.channels.PositionChan <- pos

				// Wait for channel to complete
				<-t.channels.PositionChan
			}

			t.AccountHistory = append(t.AccountHistory, *t.Account)

			// Now send the latest trade bin
			tradeUpdate := []iex.TradeBin{row}
			t.channels.TradeBinChan <- tradeUpdate

			// Wait for channel to complete
			<-t.channels.TradeBinChan
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
	t.CurrentTime = currentTime
	logger.Infof("Set current timestamp: %v\n", t.CurrentTime)
}

// Get the last account history, the first time should just return
func (t *Tantra) getLastAccountHistory() *models.Market {
	length := len(t.AccountHistory)
	if length > 0 {
		return &t.AccountHistory[length-1]
	}
	return t.Account
}

func (t *Tantra) respondWithOrderChanges() {
	for _, order := range t.newOrders {
		t.channels.OrderChan <- []iex.Order{order}
		<-t.channels.OrderChan
	}
	t.newOrders = make([]iex.Order, 0)
}

func (t *Tantra) updateBalance(currentBaseBalance float64, currentQuantity float64, averageCost float64, fillPrice float64, fillAmount float64, marketType string, updateAlgo ...bool) (float64, float64, float64) {
	logger.Infof("Updating balance with curr base bal %v, curr quant %v, avg cost %v, fill pr %v, fill a %v\n", currentBaseBalance, currentQuantity, averageCost, fillPrice, fillAmount)
	if math.Abs(fillAmount) > 0 {
		// fee := math.Abs(fillAmount/fillPrice) * t.Account.MakerFee
		// logger.Printf("fillPrice %.2f -> fillAmount %.2f", fillPrice, fillAmount)
		// logger.Debugf("Updating balance with fill cost %v, fill amount %v, qaq %v, baq %v", fillPrice, fillAmount, currentQuantity, currentBaseBalance)
		currentCost := (currentQuantity * averageCost)
		if t.marketType == exchanges.MarketType().Future {
			totalQuantity := currentQuantity + fillAmount
			newCost := fillPrice * fillAmount
			if (fillAmount >= 0 && currentQuantity >= 0) || (fillAmount <= 0 && currentQuantity <= 0) {
				//Adding to position
				averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else if ((fillAmount >= 0 && currentQuantity <= 0) || (fillAmount <= 0 && currentQuantity >= 0)) && math.Abs(fillAmount) >= math.Abs(currentQuantity) {
				//Position changed
				var diff float64
				if fillAmount > 0 {
					diff = utils.CalculateDifference(averageCost, fillPrice)
				} else {
					diff = utils.CalculateDifference(fillPrice, averageCost)
				}
				// Only use the remaining position that was filled to calculate cost
				portionFillQuantity := math.Abs(currentQuantity)
				logger.Debugf("Updating current base balance w bb %v, portionFillQuantity %v, diff %v, avgcost %v\n", currentBaseBalance, portionFillQuantity, diff, averageCost)
				currentBaseBalance = currentBaseBalance + ((portionFillQuantity * diff) / averageCost)
				averageCost = fillPrice
			} else {
				//Leaving Position
				var diff float64
				// TODO is this needed?
				// if algo.FillType == "close" {
				// 	fillPrice = t.Account.Price.Open
				// }
				// Use price open to calculate diff for filltype: close or open
				if fillAmount > 0 {
					diff = utils.CalculateDifference(averageCost, fillPrice)
				} else {
					diff = utils.CalculateDifference(fillPrice, averageCost)
				}
				logger.Debugf("Updating full fill quantity with baq %v, fillAmount %v, diff %v, avg cost %v\n", currentBaseBalance, fillAmount, diff, averageCost)
				currentBaseBalance = currentBaseBalance + ((math.Abs(fillAmount) * diff) / averageCost)
			}
			currentQuantity = currentQuantity + fillAmount
			if currentQuantity == 0 {
				averageCost = 0
			}
		} else if t.marketType == exchanges.MarketType().Spot {
			fillAmount = fillAmount / fillPrice
			totalQuantity := currentQuantity + fillAmount
			newCost := fillPrice * fillAmount

			if fillAmount >= 0 && currentQuantity >= 0 {
				//Adding to position
				averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			}

			currentQuantity = currentQuantity - newCost
			currentBaseBalance = currentBaseBalance + fillAmount
		} else if t.marketType == exchanges.MarketType().Option {
			totalQuantity := currentQuantity + fillAmount
			newCost := fillPrice * fillAmount
			if (fillAmount >= 0 && currentQuantity >= 0) || (fillAmount <= 0 && currentQuantity <= 0) {
				//Adding to position
				averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else if ((fillAmount >= 0 && currentQuantity <= 0) || (fillAmount <= 0 && currentQuantity >= 0)) && math.Abs(fillAmount) >= math.Abs(currentQuantity) {
				//Position changed
				// Only use the remaining position that was filled to calculate cost
				var balanceChange float64
				if t.Account.DenominatedInUnderlying {
					balanceChange = currentQuantity * (fillPrice - averageCost)
				} else {
					balanceChange = currentQuantity * (fillPrice - averageCost) / t.Account.Price.Close
				}
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v", currentBaseBalance, balanceChange, fillPrice, averageCost)
				currentBaseBalance = currentBaseBalance + balanceChange
				averageCost = fillPrice
			} else {
				//Leaving Position
				var balanceChange float64
				if t.Account.DenominatedInUnderlying {
					balanceChange = fillAmount * (fillPrice - averageCost)
				} else {
					balanceChange = fillAmount * (fillPrice - averageCost) / t.Account.Price.Close
				}
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v\n", currentBaseBalance, balanceChange, fillPrice, averageCost)
				currentBaseBalance = currentBaseBalance + balanceChange
			}
			currentQuantity = currentQuantity + fillAmount
		}
		if updateAlgo != nil && updateAlgo[0] {
			t.Account.BaseAsset.Quantity = currentBaseBalance
			t.Account.QuoteAsset.Quantity = currentQuantity
			t.Account.AverageCost = averageCost
		}
	}

	return currentBaseBalance, currentQuantity, averageCost
}

func (t *Tantra) getExpirys() map[int]bool {
	expirys := make(map[int]bool)
	for _, option := range t.optionContracts {
		expirys[option.Expiry] = true
	}
	return expirys
}

func (t *Tantra) GetMarkets(currency string, getMidMarket bool, marketType ...string) ([]*iex.Contract, error) {
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
		baseContract := iex.Contract{
			BaseCurrency:      t.Account.BaseAsset.Symbol,
			ContractSize:      t.Account.MinimumOrderSize,
			CreationTimestamp: utils.TimeToTimestamp(t.start),
			Expiry:            0,
			Symbol:            t.Account.Symbol,
			IsActive:          true,
			Kind:              "future",
			Leverage:          int(t.Account.MaxLeverage),
			MakerCommission:   t.Account.MakerFee,
			MinTradeAmount:    t.Account.MinimumOrderSize,
			OptionType:        "",
			Currency:          t.Account.BaseAsset.Symbol,
			SettlementPeriod:  "",
			Strike:            -1,
			TakerCommission:   t.Account.TakerFee,
			TickSize:          t.Account.TickSize,
			MidMarketPrice:    t.Account.Price.Close,
		}
		contracts = append(contracts, &baseContract)
	}
	if getOptions {
		logger.Infof("Getting markets at %v\n", t.CurrentTime)
		currentExpirys := t.getExpirys()
		logger.Debugf("Current expirys: %v\n", currentExpirys)
		newExpirys := make(map[int]bool)
		currentTime := t.CurrentTime
		for i := 0; i < t.Account.NumWeeklyOptions; i++ {
			expiry := utils.TimeToTimestamp(utils.GetNextFriday(currentTime))
			if !currentExpirys[expiry] {
				newExpirys[expiry] = true
			}
			currentTime = currentTime.Add(time.Hour * 24 * 7)
		}
		currentTime = t.CurrentTime
		for i := 0; i < t.Account.NumMonthlyOptions; i++ {
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
		if t.Account.OptionStrikeInterval == 0 {
			log.Fatalln("OptionStrikeInterval cannot be 0, does this exchange support options?")
		}
		currentPrice := *t.theoEngine.UnderlyingPrice
		minStrike := utils.RoundToNearest(currentPrice*(1+(t.Account.OptionMinStrikePct/100.)), t.Account.OptionStrikeInterval)
		maxStrike := utils.RoundToNearest(currentPrice*(1+(t.Account.OptionMaxStrikePct/100.)), t.Account.OptionStrikeInterval)
		strikes := utils.Arange(minStrike, maxStrike, t.Account.OptionStrikeInterval)
		logger.Infof("Generated strikes with current price %v, min strike %v, max strike %v, strike interval %v: %v", currentPrice, minStrike, maxStrike, t.Account.OptionStrikeInterval, strikes)
		for expiry := range newExpirys {
			for _, strike := range strikes {
				for _, optionType := range []string{"call", "put"} {
					contract := iex.Contract{
						BaseCurrency:      currency,
						ContractSize:      t.Account.OptionMinOrderSize,
						CreationTimestamp: utils.TimeToTimestamp(t.start),
						Expiry:            expiry,
						Symbol:            utils.GetDeribitOptionSymbol(expiry, strike, t.Account.BaseAsset.Symbol, optionType),
						IsActive:          true,
						Kind:              "option",
						Leverage:          1,
						MakerCommission:   t.Account.OptionSlippage,
						MinTradeAmount:    t.Account.OptionMinOrderSize,
						OptionType:        optionType,
						Currency:          currency,
						SettlementPeriod:  "",
						Strike:            strike,
						TakerCommission:   t.Account.OptionSlippage,
						TickSize:          t.Account.TickSize,
						MidMarketPrice:    -1,
					}
					contracts = append(contracts, &contract)
				}
			}
		}
	}
	logger.Debugf("Generated contracts (%v).\n", len(contracts))
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

func (t *Tantra) removeExpiredOptions() {
	currentTimestamp := utils.TimeToTimestamp(t.CurrentTime)
	for symbol, option := range t.optionContracts {
		if option.Expiry >= currentTimestamp {
			delete(t.optionContracts, symbol)
			logger.Infof("Removed expired option %v with expiry %v, currentTimestamp %v\n", option.Symbol, option.Expiry, currentTimestamp)
		}
	}
}

func (t *Tantra) parseOptionContracts(contracts []*iex.Contract) {
	currentPrice := t.Account.Price.Close
	for _, contract := range contracts {
		if contract.Kind == "option" {
			_, ok := t.optionContracts[contract.Symbol]
			if !ok {
				optionTheo := models.NewOptionTheo(
					contract.Strike,
					&currentPrice,
					contract.Expiry,
					contract.OptionType,
					t.Account.DenominatedInUnderlying,
					&t.CurrentTime,
				)
				optionTheo.CalcTheo(false)
				optionContract := models.OptionContract{
					Symbol:         contract.Symbol,
					Strike:         contract.Strike,
					Expiry:         contract.Expiry,
					OptionType:     contract.OptionType,
					AverageCost:    0,
					Profit:         0,
					Position:       0,
					OptionTheo:     &optionTheo,
					Status:         "open",
					MidMarketPrice: optionTheo.Theo,
				}
				logger.Debugf("Set mid market price for %v: %v\n", contract.Symbol, contract.MidMarketPrice)
				t.optionContracts[optionContract.Symbol] = optionContract
			}
		}
	}
	logger.Infof("Parsed %v contracts.\n", len(contracts))
}

func (t *Tantra) CurrentOptionProfit() float64 {
	currentProfit := 0.
	for _, option := range t.Account.OptionContracts {
		currentProfit += option.Profit
	}
	logger.Debugf("Got current option profit: %v\n", currentProfit)
	t.Account.OptionProfit = currentProfit
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

func (t *Tantra) getFilledBidOrders(price float64) (filledOrders []iex.Order) {
	for _, order := range t.orders {
		marketInfo, ok := t.MarketInfos[order.Market]
		if !ok {
			logger.Errorf("Order %v market is not in market infos for mock exchange.\n", order)
			continue
		}
		if marketInfo.MarketType != "option" && order.Side == "Buy" {
			if order.Type == "Market" {
				o := order // make a copy so we can delete it
				// Update Order Status
				// o.Rate :=  //TODO getMarketFillPrice()
				o.OrdStatus = t.GetPotentialOrderStatus().Filled
				filledOrders = append(filledOrders, o)
				t.newOrders = append(t.newOrders, o)
			} else if order.Rate > price {
				o := order // make a copy so we can delete it
				// Update Order Status
				o.OrdStatus = t.GetPotentialOrderStatus().Filled
				filledOrders = append(filledOrders, o)
				t.newOrders = append(t.newOrders, o)
			}
		}
	}

	// Delete filled orders from orders open
	for _, order := range filledOrders {
		delete(t.orders, order.OrderID)
	}

	return
}

func (t *Tantra) getFilledAskOrders(price float64) (filledOrders []iex.Order) {
	for _, order := range t.orders {
		marketInfo, ok := t.MarketInfos[order.Market]
		if !ok {
			logger.Errorf("Order %v market is not in market infos for mock exchange.\n", order)
			continue
		}
		if marketInfo.MarketType != "option" && order.Side == "Sell" {
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
		}
	}

	// Delete filled orders from orders open
	for _, order := range filledOrders {
		delete(t.orders, order.OrderID)
	}
	return
}

func (t *Tantra) updateOptionPositions() {
	logger.Debugf("Updating options positions with baq %v\n", t.Account.BaseAsset.Quantity)
	var optionPrice float64
	var optionQty float64
	for _, option := range t.Account.MarketStates {
		if option.MarketInfo.MarketType == "option" && option.MarketInfo.Status == "open" {
			for orderID, order := range option.Orders {
				optionPrice = order.Rate
				optionQty = order.Amount
				if order.Side == "Buy" {
					if optionPrice == 0 {
						// Simulate market order, assume theo is updated
						optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "buy", t.Account.OptionSlippage)
					}
					logger.Debugf("Updating option position for %v: position %v, price %v, qty %v\n", option.Symbol, option.Position, optionPrice, optionQty)
					option.RealizedProfit, option.Position, option.AverageCost = t.updateBalance(option.RealizedProfit, option.Position, option.AverageCost, optionPrice, optionQty, exchanges.MarketType().Option)
					// backtestDB.InsertTradeTrade(db, option.Symbol, optionPrice, optionQty, "buy", option.UnrealizedProfit, option.RealizedProfit, option.AverageCost, "market", utils.TimeToTimestamp(algo.Timestamp))
					logger.Debugf("Updated buy avgcost for option %v: %v with realized profit %v\n", option.Symbol, option.AverageCost, option.RealizedProfit)
				} else if order.Side == "Sell" {
					if optionPrice == 0 {
						// Simulate market order, assume theo is updated
						optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "sell", t.Account.OptionSlippage)
					}
					logger.Debugf("Updating option position for %v: position %v, price %v, qty %v\n", option.Symbol, option.Position, optionPrice, optionQty)
					option.RealizedProfit, option.Position, option.AverageCost = t.updateBalance(option.RealizedProfit, option.Position, option.AverageCost, optionPrice, optionQty, exchanges.MarketType().Option)
					// backtestDB.InsertTradeTrade(db, option.Symbol, optionPrice, optionQty, "buy", option.UnrealizedProfit, option.RealizedProfit, option.AverageCost, "market", utils.TimeToTimestamp(algo.Timestamp))
					logger.Debugf("Updated sell avgcost for option %v: %v with realized profit %v\n", option.Symbol, option.AverageCost, option.RealizedProfit)
				}
				delete(option.Orders, orderID)
				logger.Debugf("Removed option order with id %v.", orderID)
			}
		}
	}
}

func (t *Tantra) PlaceOrder(order iex.Order) (uuid string, err error) {
	order.TransactTime = t.CurrentTime
	log.Println("PlaceOrder", order)
	// Create uuid for order
	uuid = time.Now().String() + string(len(t.orders))
	order.OrderID = uuid
	order.OrdStatus = "Open"
	t.orders[uuid] = order
	t.newOrders = append(t.newOrders, order)
	state, ok := t.Account.MarketStates[order.Market]
	if ok {
		state.Orders[uuid] = order
	} else {
		logger.Errorf("Got order %v for unknown market: %v\n", uuid, order.Market)
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
	state, ok := t.Account.MarketStates
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
	return t.candleData[len(t.candleData)-1].Timestamp
}

func (t *Tantra) GetData(symbol string, binSize string, amount int) ([]iex.TradeBin, error) {
	// only fetch data the first time
	if t.candleData == nil {
		bars := database.GetData(symbol, t.SimulatedExchangeName, binSize, t.start, t.end)
		for i := range bars {
			tb := iex.TradeBin{
				Open:      bars[i].Open,
				High:      bars[i].High,
				Low:       bars[i].Low,
				Close:     bars[i].Close,
				Volume:    bars[i].Volume,
				Timestamp: time.Unix(bars[i].Timestamp/1000, 0),
			}
			t.candleData = append(t.candleData, tb)
		}
	}
	// after data is fetched for the first time, keep track of how many times this is called,
	// this is now the index keeper for the algo running
	// NOTE: This amount also effects the index so if it is greater than 1 it will skip through the dataset
	t.index += amount

	if t.index > amount {
		return t.candleData[t.index-amount-1 : t.index], nil
	} else {
		return t.candleData[t.index-amount : t.index], nil
	}
}

func (t *Tantra) GetBalances() (balance []iex.WSBalance, err error) {
	balance = append(balance, iex.WSBalance{
		Asset:   t.Account.BaseAsset.Symbol,
		Balance: t.Account.BaseAsset.Quantity,
	})
	return
}

func (t *Tantra) GetPositions(currency string) (positions []iex.WsPosition, err error) {
	pos := iex.WsPosition{
		Symbol:       t.Account.QuoteAsset.Symbol,
		AvgCostPrice: t.Account.AverageCost,
		CurrentQty:   t.Account.QuoteAsset.Quantity,
	}
	positions = append(positions, pos)

	for _, opt := range t.Account.OptionContracts {
		if opt.Position != 0 {
			pos := iex.WsPosition{
				Symbol:       opt.Symbol,
				AvgCostPrice: opt.AverageCost,
				CurrentQty:   opt.Position,
			}
			positions = append(positions, pos)
		}
	}
	return
}

func (t *Tantra) GetOpenOrders(vars iex.OpenOrderF) (orders []iex.Order, err error) {
	// TODO this should return currently open orders
	orders = make([]iex.Order, 0)
	return
}

//WalletHistory not available for this exchange
func (b *Tantra) GetWalletHistory(currency string) (res []iex.WalletHistoryItem, err error) {
	err = errors.New(":error: WalletHistory not available for this exchange yet")
	return
}
