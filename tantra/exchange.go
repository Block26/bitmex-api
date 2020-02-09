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
	"github.com/tantralabs/tradeapi/global/clients"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/utils"
)

// New return a instanciate bittrex struct
func New(vars iex.ExchangeConf, market models.Market) *Tantra {
	client := clients.NewClient(vars)
	var marketType string
	if market.Futures {
		marketType = exchanges.MarketType().Future
	} else {
		marketType = exchanges.MarketType().Spot
	}
	return &Tantra{
		client:                client,
		SimulatedExchangeName: vars.Exchange,
		Account:               market,
		AccountHistory:        make([]models.Market, 0),
		marketType:            marketType,
		orders:                make(map[string]iex.Order),
		newOrders:             make([]iex.Order, 0),
	}
}

// Tantra represent a Tantra client
type Tantra struct {
	client *clients.Client
	iex.IExchange
	marketType            string
	channels              *iex.WSChannels
	SimulatedExchangeName string
	Account               models.Market
	AccountHistory        []models.Market
	orders                map[string]iex.Order
	newOrders             []iex.Order
	index                 int
	data                  []iex.TradeBin
	start                 time.Time
	end                   time.Time
}

func (t *Tantra) StartWS(config interface{}) error {
	conf, ok := config.(*iex.WsConfig)
	if !ok {
		return errors.New("Assertion failed: config")
	}

	t.channels = conf.Channels
	t.index = 0

	go func() {
		for _, row := range t.data {
			// This is the start of the time step, at this point in time some events have not happened yet
			// so we will fill the orders that were placed at the end of the last time step first
			// then we we publish a new trade bin so that the algo connected can make a decision for the
			// next time interval

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

			t.AccountHistory = append(t.AccountHistory, t.Account)

			// Now send the latest trade bin
			tradeUpdate := []iex.TradeBin{row}
			t.channels.TradeBinChan <- tradeUpdate

			// Wait for channel to complete
			<-t.channels.TradeBinChan
		}
	}()

	return nil
}

// Get the last account history, the first time should just return
func (t *Tantra) getLastAccountHistory() *models.Market {
	length := len(t.AccountHistory)
	if length > 0 {
		return &t.AccountHistory[length-1]
	}
	return &t.Account
}

func (t *Tantra) respondWithOrderChanges() {
	for _, order := range t.newOrders {
		log.Println("o,", order)
	}
	if len(t.newOrders) > 0 {
		t.channels.OrderChan <- t.newOrders
		<-t.channels.OrderChan
		t.newOrders = make([]iex.Order, 0)
	}
}

func (t *Tantra) updateBalance(currentBaseBalance float64, currentQuantity float64, averageCost float64, fillPrice float64, fillAmount float64, marketType string, updateAlgo ...bool) (float64, float64, float64) {
	logger.Debugf("Updating balance with curr base bal %v, curr quant %v, avg cost %v, fill pr %v, fill a %v\n", currentBaseBalance, currentQuantity, averageCost, fillPrice, fillAmount)
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
		if order.Side == "Buy" {
			if order.Rate > price {
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
		if order.Side == "Sell" {
			if order.Rate < price {
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

func (t *Tantra) PlaceOrder(order iex.Order) (uuid string, err error) {
	log.Println("PlaceOrder", order)
	// Create uuid for order
	uuid = time.Now().String() + string(len(t.orders))
	order.OrderID = uuid
	order.OrdStatus = "Open"
	t.orders[uuid] = order
	t.newOrders = append(t.newOrders, order)
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
	return
}

func (t *Tantra) GetPotentialOrderStatus() iex.OrderStatus {
	return iex.OrderStatus{
		Filled:    "Filled",
		Rejected:  "Rejected",
		Cancelled: "Canceled",
	}
}

func (t *Tantra) GetData(symbol string, binSize string, amount int) ([]iex.TradeBin, error) {
	// only fetch data the first time
	if t.data == nil {
		t.start = time.Date(2019, 01, 01, 0, 0, 0, 0, time.UTC)
		t.end = time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)
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
			t.data = append(t.data, tb)
		}
	}
	// after data is fetched for the first time, keep track of how many times this is called,
	// this is now the index keeper for the algo running
	// NOTE: This amount also effects the index so if it is greater than 1 it will skip through the dataset
	t.index += amount

	if t.index > amount {
		return t.data[t.index-amount-1 : t.index], nil
	} else {
		return t.data[t.index-amount : t.index], nil
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
