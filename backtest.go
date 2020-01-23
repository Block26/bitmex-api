package yantra

import (
	"log"
	"math"
	"os"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/google/uuid"

	"gonum.org/v1/gonum/stat"

	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/yantra/data"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/logger"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/options"
	"github.com/tantralabs/yantra/utils"
)

// var MinimumOrderSize = 25
var currentRunUUID time.Time

var lastOptionBalance = 0.

// RunBacktest is called by passing the data set you would like to test against the algo you are testing and the current setup and rebalance functions for that algo.
// setupData will be called at the beginnning of the Backtest and rebalance will be called at every row in your dataset.
func RunBacktest(bars []*models.Bar, algo Algo, rebalance func(Algo) Algo, setupData func([]*models.Bar, Algo)) Algo {
	// Set a UUID for the run
	logger.SetLevel(algo.BacktestLogLevel)
	if currentRunUUID.IsZero() {
		currentRunUUID = time.Now()
	}

	start := time.Now()
	setupData(bars, algo)
	var history []models.History
	var timestamp time.Time
	var volData []models.ImpliedVol
	const optionLoadFreq = 15
	var lastOptionLoad int
	if algo.Market.Options {
		lastOptionLoad = 0
		volStart := int(bars[0].Timestamp) / 1000
		volEnd := int(bars[len(bars)-1].Timestamp) / 1000
		logger.Logf("Vol data start: %v, end %v", volStart, volEnd)
		algo.Timestamp = utils.TimestampToTime(volStart).String()
		volData = data.LoadImpliedVols(algo.Market.Symbol, volStart, volEnd)
		if len(volData) == 0 {
			log.Fatalln("There is no vol data in the database for", algo.Market.Symbol, "from", volStart, "to", volEnd)
		}
		algo.Market.Price = *bars[0]
		algo.Market.OptionContracts, lastOptionLoad = generateActiveOptions(lastOptionLoad, optionLoadFreq, volData, &algo)
		logger.Logf("Len vol data: %v", len(volData))
	}

	// Set contract types
	var marketType string
	if algo.Market.Futures {
		marketType = exchanges.MarketType().Future
	} else {
		marketType = exchanges.MarketType().Spot
	}
	idx := 0
	log.Println("Running", len(bars), "bars")
	for _, bar := range bars {
		if idx == 0 {
			log.Println("Start Timestamp", time.Unix(bar.Timestamp/1000, 0))
			logger.Logf("Running backtest with quote asset quantity %v and base asset quantity %v, fill type %v", algo.Market.QuoteAsset.Quantity, algo.Market.BaseAsset.Quantity, algo.FillType)
			// Set average cost if starting with a quote balance
			if algo.Market.QuoteAsset.Quantity > 0 {
				algo.Market.AverageCost = bar.Close
			}
		}
		timestamp = time.Unix(bar.Timestamp/1000, 0)
		if idx > algo.DataLength+1 {
			algo.Index = idx
			algo.Market.Price = *bar
			algo = rebalance(algo)
			if algo.FillType == exchanges.FillType().Limit {
				//Check which buys filled
				pricesFilled, ordersFilled := algo.getFilledBidOrders(bar.Low)
				fillCost, fillPercentage := algo.getCostAverage(pricesFilled, ordersFilled)
				algo.updateBalance(algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.AverageCost, fillCost, algo.Market.Buying*fillPercentage, marketType, true)
				//Check which sells filled
				pricesFilled, ordersFilled = algo.getFilledAskOrders(bar.High)
				fillCost, fillPercentage = algo.getCostAverage(pricesFilled, ordersFilled)
				algo.updateBalance(algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.AverageCost, fillCost, algo.Market.Selling*-fillPercentage, marketType, true)
			} else if algo.FillType == exchanges.FillType().Close {
				algo.updateBalanceFromFill(marketType, bar.Close)
			} else if algo.FillType == exchanges.FillType().Open {
				algo.updateBalanceFromFill(marketType, bar.Open)
			}
			if algo.Market.Options {
				algo.updateOptionPositions()
			}
			state := algo.logState(timestamp)
			history = append(history, state)
			if algo.Market.BaseAsset.Quantity <= 0 {
				logger.Logf("Ran out of balance, killing...")
				break
			}
		}
		idx++
	}

	elapsed := time.Since(start)
	log.Println("End Timestamp", timestamp)
	//TODO do this during test instead of after the test
	minProfit, maxProfit, _, maxLeverage, drawdown := minMaxStats(history)

	historyLength := len(history)
	logger.Logf("historyLength", historyLength)
	logger.Logf("Start Balance", history[0].UBalance, "End Balance", history[historyLength-1].UBalance)
	percentReturn := make([]float64, historyLength)
	last := 0.0
	for i := range history {
		if i == 0 {
			percentReturn[i] = 0
		} else {
			percentReturn[i] = utils.CalculateDifference(history[i].UBalance, last)
		}
		last = history[i].UBalance
	}

	mean, std := stat.MeanStdDev(percentReturn, nil)
	score := mean / std
	// TODO change the scoring based on 1h / 1m
	if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		score = score * math.Sqrt(365*24)
	} else if algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		score = score * math.Sqrt(365*24*60)
	}

	if math.IsNaN(score) {
		score = -100
	}

	if history[historyLength-1].Balance < 0 {
		score = -100
	}

	// logger.Logf("Last option balance: %v", lastOptionBalance)
	algo.Params["EntryOrderSize"] = algo.EntryOrderSize
	algo.Params["ExitOrderSize"] = algo.ExitOrderSize
	algo.Params["DeleverageOrderSize"] = algo.DeleverageOrderSize

	kvparams := utils.CreateKeyValuePairs(algo.Params)
	log.Printf("Balance %0.4f \n Cost %0.4f \n Quantity %0.4f \n Max Leverage %0.4f \n Max Drawdown %0.4f \n Max Profit %0.4f \n Max Position Drawdown %0.4f \n Entry Order Size %0.4f \n Exit Order Size %0.4f \n Sharpe %0.4f \n Params: %s \n",
		history[historyLength-1].Balance,
		history[historyLength-1].AverageCost,
		history[historyLength-1].Quantity,
		maxLeverage,
		drawdown,
		maxProfit,
		minProfit,
		algo.EntryOrderSize,
		algo.ExitOrderSize,
		score,
		kvparams,
	)
	log.Printf("Execution Speed: %v \n", elapsed)

	algo.Result = map[string]interface{}{
		"balance":             history[historyLength-1].UBalance,
		"max_leverage":        maxLeverage,
		"max_position_profit": maxProfit,
		"max_position_dd":     minProfit,
		"max_dd":              drawdown,
		"params":              kvparams,
		"score":               utils.ToFixed(score, 3),
	}
	//Very primitive score, how much leverage did I need to achieve this balance
	if algo.LogBacktestToCSV {
		os.Remove("balance.csv")
		historyFile, err := os.OpenFile("balance.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			panic(err)
		}
		defer historyFile.Close()

		err = gocsv.MarshalFile(&history, historyFile) // Use this to save the CSV back to the file
		if err != nil {
			panic(err)
		}
	}

	logBacktest(algo)
	logger.SetLevel(algo.LogLevel)
	return algo
}

// Core Backtest functionality
func (algo *Algo) updateBalanceFromFill(marketType string, fillPrice float64) {
	orderSize, side := algo.getOrderSize(fillPrice)
	fillCost, ordersFilled := algo.getCostAverage([]float64{fillPrice}, []float64{orderSize})
	currentWeight := math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	var fillAmount float64
	if currentWeight != float64(algo.Market.Weight) && (ordersFilled == algo.Market.Leverage || ordersFilled == algo.Market.Leverage*(-1)) {
		// Leave entire position to have quantity 0
		fillAmount = ((algo.Market.QuoteAsset.Quantity) * -1)
	} else {
		fillAmount = algo.canBuy() * (ordersFilled * side)
	}
	algo.updateBalance(algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.AverageCost, fillCost, fillAmount, marketType, true)
}

// Assume fill price is option theo adjusted for slippage
func (algo *Algo) updateOptionBalanceFromFill(option *models.OptionContract) {
	if len(option.BuyOrders.Quantity) > 0 {
		logger.Logf("Buy orders for option %v: %v", option.Symbol, option.BuyOrders)
	} else if len(option.SellOrders.Quantity) > 0 {
		logger.Logf("Sell orders for option %v: %v", option.Symbol, option.SellOrders)
	}
	for i := range option.BuyOrders.Quantity {
		optionPrice := option.BuyOrders.Price[i]
		optionQty := option.BuyOrders.Quantity[i]
		if optionPrice == 0 {
			// Simulate market order
			option.OptionTheo.CalcBlackScholesTheo(false)
			var side string
			if optionQty > 0 {
				side = "buy"
			} else if optionQty < 0 {
				side = "sell"
			}
			optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, side, algo.Market.OptionSlippage)
		}
		logger.Logf("Updating option position for %v: position %v, price %v, qty %v", option.Symbol, option.Position, optionPrice, optionQty)
		algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost = algo.updateBalance(algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost, optionPrice, optionQty, exchanges.MarketType().Option)
		logger.Logf("Updated buy avgcost for option %v: %v with baq %v", option.Symbol, option.AverageCost, algo.Market.BaseAsset.Quantity)
		option.BuyOrders = models.OrderArray{
			Quantity: []float64{},
			Price:    []float64{},
		}
		logger.Logf("Reset buy orders for %v.", option.Symbol)
	}
	for i := range option.SellOrders.Quantity {
		optionPrice := option.SellOrders.Price[i]
		optionQty := option.SellOrders.Quantity[i]
		if optionPrice == 0 {
			// Simulate market order
			option.OptionTheo.CalcBlackScholesTheo(false)
			var side string
			if optionQty > 0 {
				side = "buy"
			} else if optionQty < 0 {
				side = "sell"
			}
			optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, side, algo.Market.OptionSlippage)
		}
		logger.Logf("Updating option position for %v: position %v, price %v, qty %v", option.Symbol, option.Position, optionPrice, optionQty)
		algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost = algo.updateBalance(algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost, optionPrice, -optionQty, exchanges.MarketType().Option)
		logger.Logf("Updated sell avgcost for option %v: %v with baq %v", option.Symbol, option.AverageCost, algo.Market.BaseAsset.Quantity)
		option.SellOrders = models.OrderArray{
			Quantity: []float64{},
			Price:    []float64{},
		}
		logger.Logf("Reset sell orders for %v.", option.Symbol)
	}
}

func (algo *Algo) updateBalance(currentBaseBalance float64, currentQuantity float64, averageCost float64, fillPrice float64, fillAmount float64, marketType string, updateAlgo ...bool) (float64, float64, float64) {
	logger.Logf("Updating balance with curr base bal %v, curr quant %v, avg cost %v, fill pr %v, fill a %v", currentBaseBalance, currentQuantity, averageCost, fillPrice, fillAmount)
	if math.Abs(fillAmount) > 0 {
		// fee := math.Abs(fillAmount/fillPrice) * algo.Market.MakerFee
		// logger.Printf("fillPrice %.2f -> fillAmount %.2f", fillPrice, fillAmount)
		// logger.Logf("Updating balance with fill cost %v, fill amount %v, qaq %v, baq %v", fillPrice, fillAmount, currentQuantity, currentBaseBalance)
		currentCost := (currentQuantity * averageCost)
		if marketType == exchanges.MarketType().Future {
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
				logger.Logf("Updating current base balance w bb %v, portionFillQuantity %v, diff %v, avgcost %v", currentBaseBalance, portionFillQuantity, diff, averageCost)
				currentBaseBalance = currentBaseBalance + ((portionFillQuantity * diff) / averageCost)
				averageCost = fillPrice
			} else {
				//Leaving Position
				var diff float64
				if algo.FillType == "close" {
					fillPrice = algo.Market.Price.Open
				}
				// Use price open to calculate diff for filltype: close or open
				if fillAmount > 0 {
					diff = utils.CalculateDifference(averageCost, fillPrice)
				} else {
					diff = utils.CalculateDifference(fillPrice, averageCost)
				}
				logger.Logf("Updating full fill quantity with baq %v, fillAmount %v, diff %v, avg cost %v", currentBaseBalance, fillAmount, diff, averageCost)
				currentBaseBalance = currentBaseBalance + ((math.Abs(fillAmount) * diff) / averageCost)
			}
			currentQuantity = currentQuantity + fillAmount
			if currentQuantity == 0 {
				averageCost = 0
			}
		} else if marketType == exchanges.MarketType().Spot {
			fillAmount = fillAmount / fillPrice
			totalQuantity := currentQuantity + fillAmount
			newCost := fillPrice * fillAmount

			if fillAmount >= 0 && currentQuantity >= 0 {
				//Adding to position
				averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			}

			currentQuantity = currentQuantity - newCost
			currentBaseBalance = currentBaseBalance + fillAmount
		} else if marketType == exchanges.MarketType().Option {
			totalQuantity := currentQuantity + fillAmount
			newCost := fillPrice * fillAmount
			if (fillAmount >= 0 && currentQuantity >= 0) || (fillAmount <= 0 && currentQuantity <= 0) {
				//Adding to position
				averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else if ((fillAmount >= 0 && currentQuantity <= 0) || (fillAmount <= 0 && currentQuantity >= 0)) && math.Abs(fillAmount) >= math.Abs(currentQuantity) {
				//Position changed
				// Only use the remaining position that was filled to calculate cost
				var balanceChange float64
				if algo.Market.DenominatedInUnderlying {
					balanceChange = currentQuantity * (fillPrice - averageCost)
				} else {
					balanceChange = currentQuantity * (fillPrice - averageCost) / algo.Market.Price.Close
				}
				logger.Logf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v", currentBaseBalance, balanceChange, fillPrice, averageCost)
				currentBaseBalance = currentBaseBalance + balanceChange
				averageCost = fillPrice
			} else {
				//Leaving Position
				var balanceChange float64
				if algo.Market.DenominatedInUnderlying {
					balanceChange = fillAmount * (fillPrice - averageCost)
				} else {
					balanceChange = fillAmount * (fillPrice - averageCost) / algo.Market.Price.Close
				}
				logger.Logf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v", currentBaseBalance, balanceChange, fillPrice, averageCost)
				currentBaseBalance = currentBaseBalance + balanceChange
			}
			currentQuantity = currentQuantity + fillAmount
		}
		if updateAlgo != nil && updateAlgo[0] {
			algo.Market.BaseAsset.Quantity = currentBaseBalance
			algo.Market.QuoteAsset.Quantity = currentQuantity
			algo.Market.AverageCost = averageCost
		}
	}

	return currentBaseBalance, currentQuantity, averageCost
}

func (algo *Algo) updateOptionPositions() {
	logger.Logf("Updating options positions with baq %v", algo.Market.BaseAsset.Quantity)
	// Fill our option orders, update positions and avg costs
	for i := range algo.Market.OptionContracts {
		option := &algo.Market.OptionContracts[i]
		algo.updateOptionBalanceFromFill(option)
	}
	currentTime := utils.ToTimeObject(algo.Timestamp)
	currentTimeMillis := int(currentTime.UnixNano() / int64(time.Millisecond))
	var optionContracts []*models.OptionContract
	for i := 0; i < len(algo.Market.OptionContracts); i++ {
		optionContracts = append(optionContracts, &algo.Market.OptionContracts[i])
	}
	// Update option profit
	logger.Logf("Aggregating open option pnl...")
	options.AggregateOpenOptionPnl(optionContracts, currentTimeMillis, algo.Market.Price.Close, "BlackScholes")
	logger.Logf("Aggregating expired option pnl...")
	options.AggregateExpiredOptionPnl(optionContracts, currentTimeMillis, algo.Market.Price.Close)
}

func (algo *Algo) CurrentOptionProfit() float64 {
	currentProfit := 0.
	for _, option := range algo.Market.OptionContracts {
		currentProfit += option.Profit
	}
	logger.Logf("Got current option profit: %v", currentProfit)
	algo.Market.OptionProfit = currentProfit
	return currentProfit
}

func (algo *Algo) getCostAverage(pricesFilled []float64, ordersFilled []float64) (float64, float64) {
	// print(len(prices), len(orders), len(timestamp_arr[0]))
	percentageFilled := utils.SumArr(ordersFilled)
	if percentageFilled > 0 {
		normalizer := 1 / percentageFilled
		norm := utils.MulArr(ordersFilled, normalizer)
		costAverage := utils.SumArr(utils.MulArrs(pricesFilled, norm))
		return costAverage, percentageFilled
	}
	return 0.0, 0.0
}

func minMaxStats(history []models.History) (float64, float64, float64, float64, float64) {
	var maxProfit float64 = history[0].Profit
	var minProfit float64 = history[0].Profit

	var maxLeverage float64 = history[0].Leverage
	var minLeverage float64 = history[0].Leverage

	var maxPositionLoss float64 = 0.0
	var maxPositionProfit float64 = 0.0
	var drawdown float64 = 0.0
	var highestBalance float64 = 0.0

	for _, row := range history {
		if maxProfit < row.Profit {
			maxProfit = row.MaxProfit
			maxPositionProfit = maxProfit / row.UBalance
		}
		if minProfit > row.Profit {
			minProfit = row.MaxLoss
			maxPositionLoss = minProfit / row.UBalance
		}

		if row.UBalance > highestBalance {
			highestBalance = row.UBalance
		}

		ddDiff := utils.CalculateDifference(row.UBalance, highestBalance)
		if drawdown > ddDiff {
			drawdown = ddDiff
		}

		if maxLeverage < row.Leverage {
			maxLeverage = row.Leverage
		}
		if minLeverage > row.Leverage {
			minLeverage = row.Leverage
		}
	}
	return maxPositionLoss, maxPositionProfit, minLeverage, maxLeverage, drawdown
}

func (algo *Algo) getFilledBidOrders(price float64) ([]float64, []float64) {
	var hitPrices []float64
	var hitQuantities []float64

	var oldPrices []float64
	var oldQuantities []float64
	for i := range algo.Market.BuyOrders.Price {
		if algo.Market.BuyOrders.Price[i] > price {
			hitPrices = append(hitPrices, algo.Market.BuyOrders.Price[i])
			hitQuantities = append(hitQuantities, algo.Market.BuyOrders.Quantity[i])
		} else {
			oldPrices = append(oldPrices, algo.Market.BuyOrders.Price[i])
			oldQuantities = append(oldQuantities, algo.Market.BuyOrders.Quantity[i])
		}
	}

	algo.Market.BuyOrders.Price = oldPrices
	algo.Market.BuyOrders.Quantity = oldQuantities
	return hitPrices, hitQuantities
}

func (algo *Algo) getFilledAskOrders(price float64) ([]float64, []float64) {
	var hitPrices []float64
	var hitQuantities []float64

	var oldPrices []float64
	var oldQuantities []float64
	for i := range algo.Market.SellOrders.Price {
		if algo.Market.SellOrders.Price[i] < price {
			hitPrices = append(hitPrices, algo.Market.SellOrders.Price[i])
			hitQuantities = append(hitQuantities, algo.Market.SellOrders.Quantity[i])
		} else {
			oldPrices = append(oldPrices, algo.Market.SellOrders.Price[i])
			oldQuantities = append(oldQuantities, algo.Market.SellOrders.Quantity[i])
		}
	}

	algo.Market.SellOrders.Price = oldPrices
	algo.Market.SellOrders.Quantity = oldQuantities
	return hitPrices, hitQuantities
}

func logBacktest(algo Algo) {
	influx, _ := client.NewHTTPClient(client.HTTPConfig{
		Addr: "http://ec2-34-222-170-225.us-west-2.compute.amazonaws.com:8086",
	})

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "backtests",
		Precision: "us",
	})

	uuid := algo.Name + "-" + uuid.New().String()
	tags := map[string]string{
		"algo_name":   algo.Name,
		"run_id":      currentRunUUID.String(),
		"backtest_id": uuid,
	}

	algo.Result["id"] = uuid

	pt, _ := client.NewPoint(
		"result",
		tags,
		algo.Result,
		time.Now(),
	)
	bp.AddPoint(pt)

	client.Client.Write(influx, bp)
	influx.Close()
}

//Options backtesting functionality

// func (algo *Algo) updateOptionsPositions() {
// 	//Aggregate positions
// 	for i := range algo.Market.OptionContracts {
// 		option := &algo.Market.OptionContracts[i]
// 		total := 0.
// 		netTotal := 0.
// 		avgPrice := 0.
// 		hasAmount := false
// 		if len(option.SellOrders.Quantity) > 0 {
// 			logger.Logf("Found orders for option %v: %v", option.Symbol, option.SellOrders)
// 		}
// 		for i, qty := range option.BuyOrders.Quantity {
// 			price := option.BuyOrders.Price[i]
// 			var adjPrice float64
// 			if price > 0 {
// 				// Limit order
// 				adjPrice = utils.AdjustForSlippage(price, "buy", algo.Market.OptionSlippage)
// 			} else {
// 				// Market order
// 				if option.OptionTheo.Theo < 0 {
// 					option.OptionTheo.CalcBlackScholesTheo(false)
// 				}
// 				adjPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "buy", algo.Market.OptionSlippage)
// 			}
// 			adjPrice = utils.RoundToNearest(adjPrice, algo.Market.OptionTickSize)
// 			if adjPrice > 0 {
// 				logger.Logf("Updating avgprice with avgprice %v total %v adjprice %v qty %v", avgPrice, total, adjPrice, qty)
// 				avgPrice = ((avgPrice * total) + (adjPrice * qty)) / (total + qty)
// 				total += qty
// 				netTotal += qty
// 			} else {
// 				logger.Logf("Cannot buy option %v for adjPrice 0", option.Symbol)
// 			}
// 			hasAmount = true
// 		}
// 		for i, qty := range option.SellOrders.Quantity {
// 			price := option.SellOrders.Price[i]
// 			var adjPrice float64
// 			if price > 0 {
// 				// Limit order
// 				adjPrice = utils.AdjustForSlippage(price, "sell", .05)
// 			} else {
// 				// Market order
// 				if option.OptionTheo.Theo < 0 {
// 					option.OptionTheo.CalcBlackScholesTheo(false)
// 				}
// 				adjPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "sell", .05)
// 			}
// 			adjPrice = utils.RoundToNearest(adjPrice, algo.Market.OptionTickSize)
// 			if adjPrice > 0 {
// 				logger.Logf("Updating avgprice with avgprice %v total %v adjprice %v qty %v", avgPrice, total, adjPrice, qty)
// 				avgPrice = math.Abs(((avgPrice * total) + (adjPrice * qty)) / (total + qty))
// 				total += qty
// 				netTotal -= qty
// 			} else {
// 				logger.Logf("Cannot sell option %v for adjPrice 0", option.Symbol)
// 			}
// 			hasAmount = true
// 		}
// 		if hasAmount {
// 			//Fill open orders
// 			logger.Logf("Calcing new avg cost with avg cost %v, position %v, avgprice %v, total %v", option.AverageCost, option.Position, avgPrice, total)
// 			if option.Position+netTotal == 0 {
// 				option.Profit = option.Position * (avgPrice - option.AverageCost)
// 				logger.Logf("Net total %v closes out position %v with avgcost %v and avgprice %v, profit %v", netTotal, option.Position, option.AverageCost, avgPrice, option.Profit)
// 				option.AverageCost = 0
// 				option.Position = 0
// 			} else {
// 				option.AverageCost = ((option.AverageCost * option.Position) + (avgPrice * netTotal)) / (option.Position + netTotal)
// 				option.Position += netTotal
// 			}
// 			option.BuyOrders = models.OrderArray{
// 				Quantity: []float64{},
// 				Price:    []float64{},
// 			}
// 			option.SellOrders = models.OrderArray{
// 				Quantity: []float64{},
// 				Price:    []float64{},
// 			}
// 			logger.Logf("[%v] updated avgcost %v and position %v", option.Symbol, option.AverageCost, option.Position)
// 		}
// 	}
// }

func generateActiveOptions(lastOptionLoad int, optionLoadFreq int, volData []models.ImpliedVol, algo *Algo) ([]models.OptionContract, int) {
	if utils.ToIntTimestamp(algo.Timestamp)-lastOptionLoad < optionLoadFreq*1000 {
		return algo.Market.OptionContracts, lastOptionLoad
	}
	// logger.Logf("Generating active options with last option load %v, current timestamp %v", lastOptionLoad, utils.ToIntTimestamp(algo.Timestamp))
	//Build expirys
	var expirys []int
	currentTime := utils.ToTimeObject(algo.Timestamp)
	for i := 0; i < algo.Market.NumWeeklyOptions; i++ {
		expiry := utils.TimeToTimestamp(utils.GetNextFriday(currentTime))
		expirys = append(expirys, expiry)
		currentTime = currentTime.Add(time.Hour * 24 * 7)
	}
	currentTime = utils.ToTimeObject(algo.Timestamp)
	for i := 0; i < algo.Market.NumMonthlyOptions; i++ {
		expiry := utils.TimeToTimestamp(utils.GetLastFridayOfMonth(currentTime))
		if !utils.IntInSlice(expiry, expirys) {
			expirys = append(expirys, expiry)
		}
		currentTime = currentTime.Add(time.Hour * 24 * 28)
	}
	// logger.Logf("Generated expirys: %v", expirys)
	if algo.Market.OptionStrikeInterval == 0 {
		log.Fatalln("OptionStrikeInterval cannot be 0, does this exchange support options?")
	}
	minStrike := utils.RoundToNearest(algo.Market.Price.Close*(1+(algo.Market.OptionMinStrikePct/100.)), algo.Market.OptionStrikeInterval)
	maxStrike := utils.RoundToNearest(algo.Market.Price.Close*(1+(algo.Market.OptionMaxStrikePct/100.)), algo.Market.OptionStrikeInterval)
	strikes := utils.Arange(minStrike, maxStrike, algo.Market.OptionStrikeInterval)
	// logger.Logf("Generated strikes with current price %v min strike %v and max strike %v: %v", algo.Market.Price.Close, minStrike, maxStrike, strikes)
	var optionContracts []models.OptionContract
	for _, expiry := range expirys {
		for _, strike := range strikes {
			for _, optionType := range []string{"call", "put"} {
				vol := getNearestVol(volData, utils.ToIntTimestamp(algo.Timestamp))
				optionTheo := models.NewOptionTheo(optionType, algo.Market.Price.Close, strike, utils.ToIntTimestamp(algo.Timestamp), expiry, 0, vol, -1, algo.Market.DenominatedInUnderlying)
				optionContract := models.OptionContract{
					Symbol:         utils.GetDeribitOptionSymbol(expiry, strike, algo.Market.QuoteAsset.Symbol, optionType),
					Strike:         strike,
					Expiry:         expiry,
					OptionType:     optionType,
					AverageCost:    0,
					Profit:         0,
					Position:       0,
					OptionTheo:     *optionTheo,
					Status:         "open",
					MidMarketPrice: -1.,
				}
				optionContracts = append(optionContracts, optionContract)
			}
		}
	}
	lastOptionLoad = utils.ToIntTimestamp(algo.Timestamp)
	return optionContracts, lastOptionLoad
}

func (algo *Algo) updateActiveOptions(lastOptionLoad, optionLoadFreq int, volData []models.ImpliedVol) int {
	activeOptions, lastOptionLoad := generateActiveOptions(lastOptionLoad, optionLoadFreq, volData, algo)
	for _, activeOption := range activeOptions {
		// Check to see if this option is already known
		isNew := true
		for _, option := range algo.Market.OptionContracts {
			if option.Symbol == activeOption.Symbol {
				isNew = false
				break
			}
		}
		if isNew {
			algo.Market.OptionContracts = append(algo.Market.OptionContracts, activeOption)
			// logger.Logf("Found new active option: %v", activeOption.OptionTheo.String())
		}
	}
	return lastOptionLoad
}

func getNearestVol(volData []models.ImpliedVol, time int) float64 {
	vol := -1.
	for _, data := range volData {
		timeDiff := time - data.Timestamp
		if timeDiff < 0 {
			vol = data.IV / 100 //Assume volData quotes IV in pct
			break
		}
	}
	return vol
}
