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

	"github.com/gocarina/gocsv"
	"github.com/google/uuid"

	"gonum.org/v1/gonum/stat"

	client "github.com/influxdata/influxdb1-client/v2"
	te "github.com/tantralabs/theo-engine"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/logger"
	. "github.com/tantralabs/models"
	"github.com/tantralabs/yantra/utils"
)

// var MinimumOrderSize = 25
var currentRunUUID time.Time
var cpuprofile = flag.String("cpuprofile", "", "write cpu profile to `file`")
var memprofile = flag.String("memprofile", "", "write memory profile to `file`")
var lastOptionBalance = 0.

// RunBacktest is called by passing the data set you would like to test against the algo you are testing and the current setup and rebalance functions for that algo.
// setupData will be called at the beginnning of the Backtest and rebalance will be called at every row in your dataset.
func RunBacktest(bars []*Bar, algo Algo, rebalance func(Algo) Algo, setupData func([]*Bar, Algo)) Algo {
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
	setupData(bars, algo)
	var history []models.History
	algo.Timestamp = utils.TimestampToTime(int(bars[0].Timestamp))
	if algo.Market.Options {
		// Build theo engine
		logger.Infof("Building new theo engine at %v\n", algo.Timestamp)
		theoEngine := te.NewTheoEngine(&algo.Market, nil, &algo.Timestamp, 60000, 86400000, true, int(bars[0].Timestamp), int(bars[len(bars)-1].Timestamp))
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
			algo = rebalance(algo)
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
				algo.TheoEngine.UpdateActiveOptions()
				logger.Debugf("Updating active options took %v ns\n", time.Now().UnixNano()-start)
				start = time.Now().UnixNano()
				updateOptionPositions(&algo)
				logger.Debugf("Updating options positions took %v ns\n", time.Now().UnixNano()-start)
			}
			state := logState(&algo, timestamp)
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
	logger.Debug("Start Balance", history[0].UBalance, "End Balance", history[historyLength-1].UBalance)
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

	// logger.Debugf("Last option balance: %v", lastOptionBalance)
	if algo.AutoOrderPlacement {
		algo.Params["EntryOrderSize"] = algo.EntryOrderSize
		algo.Params["ExitOrderSize"] = algo.ExitOrderSize
		algo.Params["DeleverageOrderSize"] = algo.DeleverageOrderSize
	}

	kvparams := utils.CreateKeyValuePairs(algo.Params)
	log.Printf("Balance %0.4f \n Cost %0.4f \n Quantity %0.4f \n Max Leverage %0.4f \n Max Drawdown %0.4f \n Max Profit %0.4f \n Max Position Drawdown %0.4f \n Entry Order Size %0.4f \n Exit Order Size %0.4f \n Sharpe %0.3f \n Params: %s \n",
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
	for _, option := range algo.TheoEngine.GetOpenOptions() {
		algo.updateOptionBalanceFromFill(option)
	}
	algo.TheoEngine.UpdateOptionIndexes()
	algo.TheoEngine.ScanOptions(false, false)
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
