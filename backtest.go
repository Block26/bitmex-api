package yantra

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/fatih/structs"
	"github.com/gocarina/gocsv"
	"github.com/google/uuid"

	"gonum.org/v1/gonum/stat"

	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/exchanges"
	"github.com/tantralabs/logger"
	. "github.com/tantralabs/models"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/utils"
)

// var MinimumOrderSize = 25
var currentRunUUID time.Time
var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")
var lastOptionBalance = 0.

// RunBacktest is called by passing the data set you would like to test against the algo you are testing and the current setup and rebalance functions for that algo.
// setupData will be called at the beginnning of the Backtest and rebalance will be called at every row in your dataset.
func RunBacktest(bars []*Bar, algo Algo, rebalance func(*Algo), setupData func(*Algo, []*Bar)) Algo {
	flag.Parse()
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal("could not create CPU profile: ", err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			log.Fatal("could not start CPU profile: ", err)
		}
		defer pprof.StopCPUProfile()
	}
	// Set a UUID for the run
	if currentRunUUID.IsZero() {
		currentRunUUID = time.Now()
	}

	start := time.Now()
	algo.OHLCV = utils.GetOHLCV(bars)
	setupData(&algo, bars)
	history := make([]History, 0)
	algo.Timestamp = utils.TimestampToTime(int(bars[0].Timestamp))
	if algo.Market.Options {
		// Build theo engine
		logger.Debugf("Building new theo engine at %v\n", algo.Timestamp)
		theoEngine := te.NewTheoEngine(&algo.Market, nil, &algo.Timestamp, 60000, 86400000, true, int(bars[0].Timestamp), int(bars[len(bars)-1].Timestamp), algo.BacktestLogLevel)
		algo.TheoEngine = &theoEngine
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
			log.Println("Start Timestamp", time.Unix(bar.Timestamp/1000, 0).UTC())
			logger.Debugf("Running backtest with quote asset quantity %v and base asset quantity %v, fill type %v\n", algo.Market.QuoteAsset.Quantity, algo.Market.BaseAsset.Quantity, algo.FillType)
			// Set average cost if starting with a quote balance
			if algo.Market.QuoteAsset.Quantity > 0 {
				algo.Market.AverageCost = bar.Close
			}
		}
		algo.Timestamp = utils.TimestampToTime(int(bar.Timestamp))
		var start int64
		if idx > algo.DataLength+1 && idx < len(bars)-1 {
			algo.Index = idx
			algo.Market.Price = *bar
			start = time.Now().UnixNano()
			rebalance(&algo)
			logger.Debugf("Rebalance took %v ns\n", time.Now().UnixNano()-start)
			if algo.FillType == exchanges.FillType().Limit {
				//Check which buys filled
				pricesFilled, ordersFilled := getFilledBidOrders(&algo, bar.Low)
				fillCost, fillPercentage := getCostAverage(&algo, pricesFilled, ordersFilled)
				updateBalance(&algo, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.AverageCost, fillCost, algo.Market.Buying*fillPercentage, marketType, true)
				//Check which sells filled
				pricesFilled, ordersFilled = getFilledAskOrders(&algo, bar.High)
				fillCost, fillPercentage = getCostAverage(&algo, pricesFilled, ordersFilled)
				updateBalance(&algo, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.AverageCost, fillCost, algo.Market.Selling*-fillPercentage, marketType, true)
			} else {
				updateBalanceFromFill(&algo, marketType, getFillPrice(&algo, bars[idx+algo.FillShift]))
			}

			if algo.Market.Options {
				start = time.Now().UnixNano()
				algo.TheoEngine.(*te.TheoEngine).UpdateActiveContracts()
				logger.Debugf("Updating active options took %v ns\n", time.Now().UnixNano()-start)
				start = time.Now().UnixNano()
				updateOptionPositions(&algo)
				logger.Debugf("Updating options positions took %v ns\n", time.Now().UnixNano()-start)
			}
			state := logState(&algo, algo.Timestamp)
			history = append(history, state)
			if algo.Market.BaseAsset.Quantity <= 0 {
				logger.Debugf("Ran out of balance, killing...\n")
				break
			}
		}
		idx++
	}

	elapsed := time.Since(start)
	log.Println("End Timestamp", algo.Timestamp)
	//TODO do this during test instead of after the test
	minProfit, maxProfit, _, maxLeverage, drawdown := minMaxStats(history)

	historyLength := len(history)
	log.Println("historyLength", historyLength, "Start Balance", history[0].UBalance, "End Balance", history[historyLength-1].UBalance)
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

	if algo.AutoOrderPlacement {
		algo.Params["AutoOrderPlacement"] = map[string]interface{}{
			"EntryOrderSize":      algo.EntryOrderSize,
			"ExitOrderSize":       algo.ExitOrderSize,
			"DeleverageOrderSize": algo.DeleverageOrderSize,
		}
	}

	// logger.Debugf("Last option balance: %v", lastOptionBalance)
	// log.Println("Params1", algo.Params)
	kvparams := utils.CreateKeyValuePairs(algo.Params, true)
	log.Printf("Balance %0.4f \n Cost %0.4f \n Quantity %0.4f \n Max Leverage %0.4f \n Max Drawdown %0.4f \n Max Profit %0.4f \n Max Position Drawdown %0.4f \n Sharpe %0.3f \n Params: %s",
		history[historyLength-1].Balance,
		history[historyLength-1].AverageCost,
		history[historyLength-1].Quantity,
		maxLeverage,
		drawdown,
		maxProfit,
		minProfit,
		score,
		kvparams,
	)

	//Log turnover stats
	if algo.LogStats == true {
		stats := turnoverStats(history, algo)
		statsMap := structs.Map(stats)
		kvStats := utils.CreateKeyValuePairs(statsMap, true)

		fmt.Print("Backtested Stats")
		fmt.Printf("%s", kvStats)

		// Log stats history
		// os.Remove("stats.csv")
		// statsFile, err := os.OpenFile("stats.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
		// if err != nil {
		// 	panic(err)
		// }
		// defer statsFile.Close()

		// testStats = append(testStats, stats)

		// err = gocsv.MarshalFile(testStats, statsFile) // Use this to save the CSV back to the file
		// if err != nil {
		// 	panic(err)
		// }
	}

	fmt.Println("-------------------------------")
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
		// Log balance history
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

		// Log signal history
		os.Remove("signals.csv")
		file, err := os.OpenFile("signals.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
		if err != nil {
			panic(err)
		}
		defer file.Close()

		writer := csv.NewWriter(file)
		headers := make([]string, 0)
		rows := make([][]float64, 0)
		for key, values := range algo.Signals {
			headers = append(headers, key)
			rows = append(rows, values)
		}

		if len(rows) > 0 {
			r := make([]string, 0, 1+len(headers))
			r = append(
				r,
				headers...,
			)
			writer.Write(r)
			for i := range rows[0] {
				r := make([]string, 0, 1+len(headers))
				vals := make([]string, 0, 1+len(headers))
				for x := range headers {
					vals = append(vals, fmt.Sprintf("%0.8f", rows[x][i]))
				}
				r = append(
					r,
					vals...,
				)
				writer.Write(r)
			}
		}

		writer.Flush()
	}

	logBacktest(algo)

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			log.Fatal("could not create memory profile: ", err)
		}
		defer f.Close()
		runtime.GC() // get up-to-date statistics
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("could not write memory profile: ", err)
		}
	}
	return algo
}

// Core Backtest functionality
func updateBalanceFromFill(algo *Algo, marketType string, fillPrice float64) {
	currentWeight := math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	orderSize, side := getOrderSize(algo, fillPrice)
	fillCost, ordersFilled := getCostAverage(algo, []float64{fillPrice}, []float64{orderSize})
	var fillAmount float64
	if currentWeight != float64(algo.Market.Weight) && (ordersFilled == algo.Market.Leverage || ordersFilled == algo.Market.Leverage*(-1)) {
		// Leave entire position to have quantity 0
		fillAmount = ((algo.Market.QuoteAsset.Quantity) * -1)
	} else {
		fillAmount = canBuy(algo) * (ordersFilled * side)
	}

	orderSide := math.Copysign(1, fillAmount)
	if orderSide == 1 {
		fillCost = fillCost * (1 + algo.Market.Slippage)
	} else if orderSide == -1 {
		fillCost = fillCost * (1 - algo.Market.Slippage)
	}

	algo.FillPrice = fillCost
	updateBalance(algo, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.AverageCost, fillCost, fillAmount, marketType, true)
}

// Assume fill price is option theo adjusted for slippage
func updateOptionBalanceFromFill(algo *Algo, option *OptionContract) {
	if len(option.BuyOrders.Quantity) > 0 {
		logger.Debugf("[%v] Buy orders for option %v: %v\n", algo.Timestamp, option.Symbol, option.BuyOrders)
	} else if len(option.SellOrders.Quantity) > 0 {
		logger.Debugf("[%v] Sell orders for option %v: %v\n", algo.Timestamp, option.Symbol, option.SellOrders)
	}
	for i := range option.BuyOrders.Quantity {
		optionPrice := option.BuyOrders.Price[i]
		optionQty := option.BuyOrders.Quantity[i]
		if optionPrice == 0 {
			// Simulate market order, assume theo is updated
			optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "buy", algo.Market.OptionSlippage)
		}
		logger.Debugf("Updating option position for %v: position %v, price %v, qty %v\n", option.Symbol, option.Position, optionPrice, optionQty)
		algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost = updateBalance(algo, algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost, optionPrice, optionQty, exchanges.MarketType().Option)
		logger.Debugf("Updated buy avgcost for option %v: %v with baq %v\n", option.Symbol, option.AverageCost, algo.Market.BaseAsset.Quantity)
		option.BuyOrders = OrderArray{
			Quantity: []float64{},
			Price:    []float64{},
		}
		logger.Debugf("Reset buy orders for %v.", option.Symbol)
	}
	for i := range option.SellOrders.Quantity {
		optionPrice := option.SellOrders.Price[i]
		optionQty := option.SellOrders.Quantity[i]
		if optionPrice == 0 {
			// Simulate market order, assume theo is updated
			optionPrice = utils.AdjustForSlippage(option.OptionTheo.Theo, "sell", algo.Market.OptionSlippage)
		}
		logger.Debugf("Updating option position for %v: position %v, price %v, qty %v\n", option.Symbol, option.Position, optionPrice, optionQty)
		algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost = updateBalance(algo, algo.Market.BaseAsset.Quantity, option.Position, option.AverageCost, optionPrice, -optionQty, exchanges.MarketType().Option)
		logger.Debugf("Updated sell avgcost for option %v: %v with baq %v\n", option.Symbol, option.AverageCost, algo.Market.BaseAsset.Quantity)
		option.SellOrders = OrderArray{
			Quantity: []float64{},
			Price:    []float64{},
		}
		logger.Debugf("Reset sell orders for %v.\n", option.Symbol)
	}
}

func updateBalance(algo *Algo, currentBaseBalance float64, currentQuantity float64, averageCost float64, fillPrice float64, fillAmount float64, marketType string, updateAlgo ...bool) (float64, float64, float64) {
	logger.Debugf("Updating balance with curr base bal %v, curr quant %v, avg cost %v, fill pr %v, fill a %v\n", currentBaseBalance, currentQuantity, averageCost, fillPrice, fillAmount)
	if math.Abs(fillAmount) > 0 {
		// fee := math.Abs(fillAmount/fillPrice) * algo.Market.MakerFee
		// logger.Printf("fillPrice %.2f -> fillAmount %.2f", fillPrice, fillAmount)
		// logger.Debugf("Updating balance with fill cost %v, fill amount %v, qaq %v, baq %v", fillPrice, fillAmount, currentQuantity, currentBaseBalance)
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
				logger.Debugf("Updating current base balance w bb %v, portionFillQuantity %v, diff %v, avgcost %v\n", currentBaseBalance, portionFillQuantity, diff, averageCost)
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
				logger.Debugf("Updating full fill quantity with baq %v, fillAmount %v, diff %v, avg cost %v\n", currentBaseBalance, fillAmount, diff, averageCost)
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
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v", currentBaseBalance, balanceChange, fillPrice, averageCost)
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
				logger.Debugf("Updating current base balance w bb %v, balancechange %v, fillprice %v, avgcost %v\n", currentBaseBalance, balanceChange, fillPrice, averageCost)
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

func updateOptionPositions(algo *Algo) {
	logger.Debugf("Updating options positions with baq %v\n", algo.Market.BaseAsset.Quantity)
	// Fill our option orders, update positions and avg costs
	for _, option := range algo.TheoEngine.(*te.TheoEngine).GetOpenOptions() {
		updateOptionBalanceFromFill(algo, option)
	}
	algo.TheoEngine.(*te.TheoEngine).UpdateOptionIndexes()
	algo.TheoEngine.(*te.TheoEngine).ScanOptions(false, false)
}

// Delete all expired options without profit values to conserve time and space resources
func removeExpiredOptions(algo *Algo) {
	numOptions := len(algo.Market.OptionContracts)
	i := 0
	for _, option := range algo.Market.OptionContracts {
		if option.Status == "expired" && option.Profit != 0 {
			algo.Market.OptionContracts[i] = option
			i++
		}
	}
	algo.Market.OptionContracts = algo.Market.OptionContracts[:i]
	logger.Debugf("Removed %v expired option contracts.\n", numOptions-len(algo.Market.OptionContracts))
}

func CurrentOptionProfit(algo *Algo) float64 {
	currentProfit := 0.
	for _, option := range algo.Market.OptionContracts {
		currentProfit += option.Profit
	}
	logger.Debugf("Got current option profit: %v\n", currentProfit)
	algo.Market.OptionProfit = currentProfit
	return currentProfit
}

func getCostAverage(algo *Algo, pricesFilled []float64, ordersFilled []float64) (float64, float64) {
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

func minMaxStats(history []History) (float64, float64, float64, float64, float64) {
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

func getFilledBidOrders(algo *Algo, price float64) ([]float64, []float64) {
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

func turnoverStats(history []History, algo Algo) Stats {
	var weight int = history[0].Weight
	var averageDailyWeightChanges float64
	var previousQuantity float64
	var previousBalance float64

	var weightChanges []int
	var profitableDays []float64
	var longPositions []float64
	var shortPositions []float64
	var currentLongPosition []float64
	var currentShortPosition []float64
	var currentLongProfit []float64
	var currentShortProfit []float64

	var longPositionsArr [][]float64
	var shortPositionsArr [][]float64
	var longPositionsProfitArr [][]float64
	var shortPositionsProfitArr [][]float64

	var totalAbsShortProfit float64 = 0.0
	var totalAbsLongProfit float64 = 0.0

	//Get weight/position/turnover stats
	for _, row := range history {
		if algo.FillType != "limit" {
			//How many times did weight change
			if weight != row.Weight {
				weight = row.Weight
				weightChanges = append(weightChanges, weight)
			}
		}
		// How many hours are we profitable //
		if history[0].Balance < row.Balance {
			profitableDays = append(profitableDays, 1.0)
		} else {
			profitableDays = append(profitableDays, 0.0)
		}
		//How many long positions did we hold //
		if previousQuantity > 0 && row.Quantity <= 0 {
			longPositions = append(longPositions, previousQuantity)
		}
		//How many short positions did we hold //
		if previousQuantity < 0 && row.Quantity >= 0 {
			shortPositions = append(shortPositions, previousQuantity)
		}
		//Create array of long positions //
		if row.Quantity > 0 {
			currentLongPosition = append(currentLongPosition, row.Quantity)
		} else {
			if len(currentLongPosition) != 0 {
				longPositionsArr = append(longPositionsArr, currentLongPosition)
				currentLongPosition = nil
			}
		}
		//Create array of short positions //
		if row.Quantity < 0 {
			currentShortPosition = append(currentShortPosition, row.Quantity)
		} else {
			if len(currentShortPosition) != 0 {
				shortPositionsArr = append(shortPositionsArr, currentShortPosition)
				currentShortPosition = nil
			}
		}
		// Create arrays of realized long position profit //
		// TODO differentiate between abs and percent profit
		if row.Quantity >= 0 && row.Quantity < previousQuantity {
			totalAbsLongProfit += row.Balance - previousBalance
			currentLongProfit = append(currentLongProfit, (row.Balance-previousBalance)/previousBalance)
			// currentLongProfit = append(currentLongProfit, row.PercentProfit)
		} else {
			// TODO also calculate exits where we get out over 5 hours, len(currentLongProfit) >= 5
			if len(currentLongProfit) != 0 {
				longPositionsProfitArr = append(longPositionsProfitArr, currentLongProfit)
				currentLongProfit = nil
			}
		}
		//Create arrays of realized short position profit //
		// TODO differentiate between abs and percent profit
		if row.Quantity <= 0 && row.Quantity > previousQuantity {
			totalAbsShortProfit += row.Balance - previousBalance
			currentShortProfit = append(currentShortProfit, (row.Balance-previousBalance)/previousBalance)
			// currentShortProfit = append(currentShortProfit, row.PercentProfit)
		} else {
			// TODO also calculate exits where we get out over 5 hours, len(currentShortProfit) >= 5
			if len(currentShortProfit) != 0 {
				shortPositionsProfitArr = append(shortPositionsProfitArr, currentShortProfit)
				currentShortProfit = nil
			}
		}
		previousQuantity = row.Quantity
		previousBalance = row.Balance
	}
	var longDurationArr []float64
	var shortDurationArr []float64
	var longProfitArr []float64
	var shortProfitArr []float64

	var currentLongLength float64
	var currentShortLength float64
	var longProfit float64
	var longWinRate float64
	var shortProfit float64
	var shortWinRate float64

	// Find Duration of Long Positions, Assumes Rebalance interval is hourly //
	for _, value := range longPositionsArr {
		currentLongLength = float64(len(value))
		longDurationArr = append(longDurationArr, currentLongLength)
		currentLongLength = 0
	}
	averageLongDuration := utils.SumArr(longDurationArr) / float64(len(longDurationArr))
	//fmt.Println("-------------------------------")
	//fmt.Println("Total Long Positions:", len(longPositions))
	//fmt.Printf("Average Long Position Duration: %0.2f hours \n", averageLongDuration)
	algo.Stats.TotalLongPositions = len(longPositions)
	algo.Stats.AverageLongPositionDuration = averageLongDuration
	// Find long position hit rate, assumes the process of exiting position is one trade //
	for _, value := range longPositionsProfitArr {
		longProfit = utils.SumArr(value)
		longProfitArr = append(longProfitArr, longProfit)
		longProfit = 0.0
	}
	//fmt.Printf("Average Long Position Profit: %0.4f \n", utils.SumArr(longProfitArr)/float64(len(longProfitArr)))
	algo.Stats.AverageLongPositionProfit = utils.SumArr(longProfitArr) / float64(len(longProfitArr))
	// Calculate Winning and Losing Long Trades //
	winningLongTrade := make([]float64, 0)
	losingLongTrade := make([]float64, 0)
	for _, x := range longProfitArr {
		if x > 0 {
			winningLongTrade = append(winningLongTrade, x)
		} else {
			losingLongTrade = append(losingLongTrade, x)
		}
	}
	longWinRate = float64(len(winningLongTrade)) / float64(len(longPositionsProfitArr))
	averageLongWin := utils.SumArr(winningLongTrade) / float64(len(winningLongTrade))
	averageLongLoss := utils.SumArr(losingLongTrade) / float64(len(losingLongTrade))
	longRiskRewardRatio := averageLongWin / math.Abs(averageLongLoss)
	requiredLongWinRate := 1 / (1 + longRiskRewardRatio)
	// fmt.Printf("Average Long Winning Position Profit: %0.4f \n", averageLongWin)
	// fmt.Printf("Average Long Losing Position Loss: %0.4f \n", averageLongLoss)
	// fmt.Printf("Risk-Reward Ratio: 1:%0.4f \n", longRiskRewardRatio)
	// fmt.Printf("How Often Do I Have to be Right: %0.4f \n", requiredLongWinRate)
	// fmt.Printf("Total Profitable Exit Trades: %d \n", len(winningLongTrade))
	// fmt.Printf("Total Exit Trades: %d \n", len(longProfitArr))
	// fmt.Printf("Long Win Rate: %0.4f \n", longWinRate)
	algo.Stats.AverageLongWinningPositionProfit = averageLongWin
	algo.Stats.AverageLongLosingPositionLoss = averageLongLoss
	// Risk Reward value to be inputed into 1:longRiskRewardRatio //
	algo.Stats.LongRiskReward = longRiskRewardRatio
	algo.Stats.LongWinsNeeded = requiredLongWinRate
	algo.Stats.TotalLongProfitableExitTrades = len(winningLongTrade)
	algo.Stats.TotalLongExitTrades = len(longProfitArr)
	algo.Stats.LongWinRate = longWinRate
	// fmt.Println("-------------------------------")

	//Find Duration of Short Positions, Assumes Rebalance interval is hourly
	for _, value := range shortPositionsArr {
		currentShortLength = float64(len(value))
		shortDurationArr = append(shortDurationArr, currentShortLength)
		currentShortLength = 0
	}
	averageShortDuration := utils.SumArr(shortDurationArr) / float64(len(shortDurationArr))
	// fmt.Println("Total Short Positions:", len(shortPositions))
	// fmt.Printf("Average Short Position Duration: %0.2f hours \n", averageShortDuration)
	algo.Stats.TotalShortPositions = len(shortPositions)
	algo.Stats.AverageShortPositionDuration = averageShortDuration

	//Find short position hit rate, assumes the process of exiting position is one trade
	for _, value := range shortPositionsProfitArr {
		shortProfit = utils.SumArr(value)
		shortProfitArr = append(shortProfitArr, shortProfit)
		shortProfit = 0.0
	}
	// log.Println("Sum Short Position Arr", shortProfitArr)
	// fmt.Printf("Average Short Position Profit: %0.4f \n", utils.SumArr(shortProfitArr)/float64(len(shortProfitArr)))
	algo.Stats.AverageShortPositionProfit = utils.SumArr(shortProfitArr) / float64(len(shortProfitArr))
	// Calculate Winning and Losing Short Trades //
	winningShortTrade := make([]float64, 0)
	losingShortTrade := make([]float64, 0)
	for _, x := range shortProfitArr {
		if x > 0 {
			winningShortTrade = append(winningShortTrade, x)
		} else {
			losingShortTrade = append(losingShortTrade, x)
		}
	}
	shortWinRate = float64(len(winningShortTrade)) / float64(len(shortPositionsProfitArr))
	averageShortWin := utils.SumArr(winningShortTrade) / float64(len(winningShortTrade))
	averageShortLoss := utils.SumArr(losingShortTrade) / float64(len(losingShortTrade))
	shortRiskRewardRatio := averageShortWin / math.Abs(averageShortLoss)
	requiredShortWinRate := 1 / (1 + shortRiskRewardRatio)
	// fmt.Printf("Average Short Winning Position Profit: %0.4f \n", averageShortWin)
	// fmt.Printf("Average Short Losing Position Loss: %0.4f \n", averageShortLoss)
	// fmt.Printf("Risk-Reward Ratio: 1:%0.4f \n", shortRiskRewardRatio)
	// fmt.Printf("How Often Do I Have to be Right: %0.4f \n", requiredShortWinRate)
	// fmt.Printf("Total Profitable Exit Trades: %d \n", len(winningShortTrade))
	// fmt.Printf("Total Exit Trades: %d \n", len(shortProfitArr))
	// fmt.Printf("Short Win Rate: %0.4f \n", shortWinRate)
	algo.Stats.AverageShortWinningPositionProfit = averageShortWin
	algo.Stats.AverageShortLosingPositionLoss = averageShortLoss
	// Risk Reward value to be inputed into 1:shortRiskRewardRatio //
	algo.Stats.ShortRiskReward = shortRiskRewardRatio
	algo.Stats.ShortWinsNeeded = requiredShortWinRate
	algo.Stats.TotalShortProfitableExitTrades = len(winningShortTrade)
	algo.Stats.TotalShortExitTrades = len(shortProfitArr)
	algo.Stats.ShortWinRate = shortWinRate
	// fmt.Println("-------------------------------")

	// Calculate total positions metrics, irrespective of side
	totalPositionProfit := []float64{}
	totalPositionProfit = append(longProfitArr, shortProfitArr...)
	winningTrade := make([]float64, 0)
	losingTrade := make([]float64, 0)
	for _, x := range totalPositionProfit {
		if x > 0 {
			winningTrade = append(winningTrade, x)
		} else {
			losingTrade = append(losingTrade, x)
		}
	}
	averageWin := utils.SumArr(winningTrade) / float64(len(winningTrade))
	averageLoss := utils.SumArr(losingTrade) / float64(len(losingTrade))
	riskRewardRatio := averageWin / math.Abs(averageLoss)
	requiredWinRate := 1 / (1 + riskRewardRatio)
	// fmt.Printf("Total Positions: %d \n", (len(longPositions) + len(shortPositions)))
	// fmt.Printf("Total Position Average Profit: %0.4f \n", utils.SumArr(totalPositionProfit)/float64(len(totalPositionProfit)))
	// fmt.Printf("Average Winning Position Profit: %0.4f \n", averageWin)
	// fmt.Printf("Average Losing Position Loss: %0.4f \n", averageLoss)
	// fmt.Printf("Risk-Reward Ratio: 1:%0.4f \n", riskRewardRatio)
	// fmt.Printf("How Often Do I Have to be Right: %0.4f \n", requiredWinRate)
	// fmt.Printf("Total Win Rate: %0.4f \n", (float64(len(winningShortTrade)+len(winningLongTrade)))/float64((len(shortProfitArr)+len(longProfitArr))))
	algo.Stats.TotalPositions = (len(longPositions) + len(shortPositions))
	algo.Stats.AveragePositionProfit = utils.SumArr(totalPositionProfit) / float64(len(totalPositionProfit))
	algo.Stats.AverageWinningPositionProfit = averageWin
	algo.Stats.AverageLosingPositionLoss = averageLoss
	// Risk Reward value to be inputed into 1:riskRewardRatio //
	algo.Stats.TotalRiskReward = riskRewardRatio
	algo.Stats.TotalWinsNeeded = requiredWinRate
	algo.Stats.TotalWinRate = (float64(len(winningShortTrade) + len(winningLongTrade))) / float64((len(shortProfitArr) + len(longProfitArr)))

	// Find Average Daily Weight Changes, Assumes Rebalance interval is hourly
	numberOfDays := float64((len(history) / 24))
	totalChanges := float64(len(weightChanges))
	averageDailyWeightChanges = totalChanges / numberOfDays
	algo.Stats.AverageDailyWeightChanges = averageDailyWeightChanges
	percentDaysProfitable := utils.SumArr(profitableDays) / float64(len(profitableDays))
	// fmt.Printf("Percent Days Profitable: %0.4f \n", percentDaysProfitable)
	algo.Stats.PercentDaysProfitable = percentDaysProfitable
	return algo.Stats
}

func getFilledAskOrders(algo *Algo, price float64) ([]float64, []float64) {
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
	influxURL := os.Getenv("YANTRA_BACKTEST_DB_URL")
	if influxURL == "" {
		log.Fatalln("You need to set the `YANTRA_BACKTEST_DB_URL` env variable")
	}

	influxUser := os.Getenv("YANTRA_BACKTEST_DB_USERNAME")
	influxPassword := os.Getenv("YANTRA_BACKTEST_DB_PASSWORD")

	influx, _ := client.NewHTTPClient(client.HTTPConfig{
		Addr:     influxURL,
		Username: influxUser,
		Password: influxPassword,
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
