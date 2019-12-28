package algo

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/google/uuid"

	"gonum.org/v1/gonum/stat"

	"github.com/gocarina/gocsv"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/TheAlgoV2/models"

	"github.com/tantralabs/TheAlgoV2/tantradb"
	. "gopkg.in/src-d/go-git.v4/_examples"
)

// var MinimumOrderSize = 25
var currentRunUUID time.Time

var VolData []models.ImpliedVol
var lastOptionBalance = 0.

//TODO: these should be a config somewhere
const StrikeInterval = 250.
const TickSize = .1
const MinTradeAmount = .1
const MakerFee = 0.
const TakerFee = .001

var LastOptionLoad = 0
var OptionLoadFreq = 86400

func RunBacktest(data []*models.Bar, algo Algo, rebalance func(float64, Algo) Algo, setupData func([]*models.Bar, Algo)) Algo {
	setupData(data, algo)
	start := time.Now()
	var history []models.History
	var timestamp time.Time
	// starting_algo.Market.BaseBalance := 0
	volStart := ToIntTimestamp(data[0].Timestamp)
	volEnd := ToIntTimestamp(data[len(data)-1].Timestamp)
	fmt.Printf("Vol data start: %v, end %v\n", volStart, volEnd)
	algo.Timestamp = data[0].Timestamp
	VolData = tantradb.LoadImpliedVols("XBTUSD", volStart, volEnd)
	algo.Market.Options = generateActiveOptions(&algo)
	fmt.Printf("Len vol data: %v\n", len(VolData))
	timestamp := ""
	idx := 0
	log.Println("Running", len(data), "bars")
	for _, bar := range data {
		if idx == 0 {
			log.Println("Start Timestamp", bar.Timestamp)
			// 	//Set average cost if starting with a quote balance
			if algo.Market.QuoteAsset.Quantity > 0 {
				algo.Market.AverageCost = bar.Close
			}
		}
		timestamp = bar.Timestamp
		if idx > algo.DataLength+1 {
			algo.Index = idx
			algo.Market.Price = *bar
			// algo.updateActiveOptions()
			algo = rebalance(bar.Open, algo)
			// log.Println(data)
			if algo.FillType == "limit" {
				//Check which buys filled
				pricesFilled, ordersFilled := getFilledBidOrders(algo.Market.BuyOrders.Price, algo.Market.BuyOrders.Quantity, bar.Low)
				fillCost, fillPercentage := algo.getCostAverage(pricesFilled, ordersFilled)
				algo.UpdateBalance(fillCost, algo.Market.Buying*fillPercentage)

				//Check which sells filled
				pricesFilled, ordersFilled = getFilledAskOrders(algo.Market.SellOrders.Price, algo.Market.SellOrders.Quantity, bar.High)
				fillCost, fillPercentage = algo.getCostAverage(pricesFilled, ordersFilled)
				algo.UpdateBalance(fillCost, algo.Market.Selling*-fillPercentage)
				algo.updateOptionsPositions()
			} else if algo.FillType == "close" {
				algo.updateBalanceFromFill(bar.Close)
			} else if algo.FillType == "open" {
				algo.updateBalanceFromFill(bar.Open)
			}
			// updateBalanceXBTStrat(bar)
			state := algo.logState(timestamp)
			history = append(history, state)
			// if algo.Market.Qua+(algo.Market.BaseBalance*algo.Market.Profit) < 0 {
			// 	break
			// }
			// history.Balance[len(history.Balance)-1], == portfolio value
			// portfolioValue := history.Balance[len(history.Balance)-1]
		}
		idx++
	}

	elapsed := time.Since(start)
	log.Println("End Timestamp", timestamp)
	//TODO do this during test instead of after the test
	minProfit, maxProfit, _, maxLeverage, drawdown := MinMaxStats(history)
	// score := (history[historyLength-1].Balance) + drawdown*3 //+ (minProfit * maxLeverage) - drawdown // maximize
	// score := (history[historyLength-1].Balance) * math.Abs(1/drawdown) //+ (minProfit * maxLeverage) - drawdown // maximize

	historyLength := len(history)
	log.Println("historyLength", historyLength)
	log.Println("Start Balance", history[0].UBalance, "End Balance", history[historyLength-1].UBalance)
	percentReturn := make([]float64, historyLength)
	last := 0.0
	for i := range history {
		if i == 0 {
			percentReturn[i] = 0
		} else {
			percentReturn[i] = calculateDifference(history[i].UBalance, last)
		}
		last = history[i].UBalance
	}

	mean, std := stat.MeanStdDev(percentReturn, nil)
	// log.Println("mean", mean, "std", std)
	score := mean / std
	// TODO change the scoring based on 1h / 1m
	score = score * math.Sqrt(365*24*60)

	if math.IsNaN(score) {
		score = -100
	}

	if history[historyLength-1].Balance < 0 {
		score = -100
	}

	// fmt.Printf("Last option balance: %v\n", lastOptionBalance)

	kvparams := createKeyValuePairs(algo.Params)
	fmt.Printf("Balance %0.4f \n Cost %0.4f \n Quantity %0.4f \n Max Leverage %0.4f \n Max Drawdown %0.4f \n Max Profit %0.4f \n Max Position Drawdown %0.4f \n Entry Order Size %0.4f \n Exit Order Size %0.4f \n Sharpe %0.4f \n Params: %s",
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
	log.Println("Execution Speed", elapsed)
	// log.Println("History Length", len(history), "Start Balance", history[0].UBalance, "End Balance", history[historyLength-1].UBalance)

	algo.Params["EntryOrderSize"] = algo.EntryOrderSize
	algo.Params["ExitOrderSize"] = algo.ExitOrderSize
	algo.Result = map[string]interface{}{
		"balance":             history[historyLength-1].UBalance,
		"max_leverage":        maxLeverage,
		"max_position_profit": maxProfit,
		"max_position_dd":     minProfit,
		"max_dd":              drawdown,
		"params":              kvparams,
		"score":               score,
	}
	//Very primitive score, how much leverage did I need to achieve this balance
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

	// LogBacktest(algo)
	// score := ((math.Abs(minProfit) / history[historyLength-1].Balance) + maxLeverage) - history[historyLength-1].Balance // minimize
	return algo //history.Balance[len(history.Balance)-1] / (maxLeverage + 1)
}

func (algo *Algo) updateBalanceFromFill(fillPrice float64) {
	orderSize, side := algo.getOrderSize(fillPrice)
	fillCost, ordersFilled := algo.getCostAverage([]float64{fillPrice}, []float64{orderSize})
	algo.UpdateBalance(fillCost, ordersFilled*side)
}

func (algo *Algo) UpdateBalance(fillCost float64, fillAmount float64) {
	// log.Printf("fillCost %.2f -> fillAmount %.2f\n", fillCost, fillCost*fillAmount)
	if math.Abs(fillAmount) > 0 {
		// fee := math.Abs(fillAmount/fillCost) * algo.Market.MakerFee
		currentCost := (algo.Market.QuoteAsset.Quantity * algo.Market.AverageCost)
		var newQuantity float64
		// log.Printf("fillCost %.8f -> fillAmount %.8f -> newQuantity %0.8f\n", fillCost, fillAmount, newQuantity)
		if algo.Market.Futures {
			newQuantity := algo.canBuy() * fillAmount
			currentWeight := math.Copysign(1, algo.Market.QuoteAsset.Quantity)
			// Leave entire position to have quantity 0
			if currentWeight != float64(algo.Market.Weight) && (fillAmount == algo.Market.Leverage || fillAmount == algo.Market.Leverage*(-1)) {
				newQuantity = ((algo.Market.QuoteAsset.Quantity) * -1)
			}
			totalQuantity := algo.Market.QuoteAsset.Quantity + newQuantity
			newCost := fillCost * newQuantity
			if (newQuantity >= 0 && algo.Market.QuoteAsset.Quantity >= 0) || (newQuantity <= 0 && algo.Market.QuoteAsset.Quantity <= 0) {
				//Adding to position
				algo.Market.AverageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else if ((newQuantity >= 0 && algo.Market.QuoteAsset.Quantity <= 0) || (newQuantity <= 0 && algo.Market.QuoteAsset.Quantity >= 0)) && math.Abs(newQuantity) >= math.Abs(algo.Market.QuoteAsset.Quantity) {
				//Position changed
				var diff float64
				if fillAmount > 0 {
					diff = calculateDifference(algo.Market.AverageCost, fillCost)
				} else {
					diff = calculateDifference(fillCost, algo.Market.AverageCost)
				}
				// Only use the remaining position that was filled to calculate cost
				portionFillQuantity := math.Abs(algo.Market.QuoteAsset.Quantity)
				algo.Market.BaseAsset.Quantity = algo.Market.BaseAsset.Quantity + ((portionFillQuantity * diff) / algo.Market.AverageCost)
				algo.Market.AverageCost = fillCost
			} else {
				//Leaving Position
				var diff float64
				if algo.FillType == "close" {
					fillCost = algo.Market.Price.Open
				}
				// Use price open to calculate diff for filltype: close or open
				if fillAmount > 0 {
					diff = calculateDifference(algo.Market.AverageCost, fillCost)
				} else {
					diff = calculateDifference(fillCost, algo.Market.AverageCost)
				}
				algo.Market.BaseAsset.Quantity = algo.Market.BaseAsset.Quantity + ((math.Abs(newQuantity) * diff) / algo.Market.AverageCost)
			}
			algo.Market.QuoteAsset.Quantity = algo.Market.QuoteAsset.Quantity + newQuantity
		} else {
			newQuantity = fillAmount / fillCost
			totalQuantity := algo.Market.QuoteAsset.Quantity + newQuantity
			newCost := fillCost * newQuantity

			if newQuantity >= 0 && algo.Market.QuoteAsset.Quantity >= 0 {
				//Adding to position
				algo.Market.AverageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			}

			algo.Market.QuoteAsset.Quantity = algo.Market.QuoteAsset.Quantity - newCost
			algo.Market.BaseAsset.Quantity = algo.Market.BaseAsset.Quantity + newQuantity

			// log.Println("PV:", (algo.Market.BaseAsset.Quantity*algo.Market.Price)+algo.Market.QuoteAsset.Quantity)
			// log.Println("Base", algo.Market.BaseAsset.Quantity, "Quote", algo.Market.QuoteAsset.Quantity)
		}
		// log.Printf("fillCost %.8f -> fillAmount %.8f -> newQuantity %0.8f\n", fillCost, fillAmount, newQuantity)
		// algo.Market.BaseAsset.Quantity = algo.Market.BaseAsset.Quantity - fee
	}
	algo.updateOptionBalance()
}

func (algo *Algo) updateOptionBalance() {
	optionBalance := 0.
	for _, option := range algo.Market.OptionContracts {
		// Calculate unrealized pnl
		option.OptionTheo.UnderlyingPrice = algo.Market.Price.Close
		option.OptionTheo.CalcBlackScholesTheo(false)
		optionBalance += option.Position * (option.OptionTheo.Theo - option.AverageCost)
		// fmt.Printf("%v with underlying price %v theo %v\n", option.OptionTheo.String(), algo.Market.Price, option.OptionTheo.Theo)
		// if OptionModel == "blackScholes" {
		// 	option.OptionTheo.CalcBlackScholesTheo(false)
		// 	optionBalance += option.Position * (option.OptionTheo.Theo - option.AverageCost)
		// } else if OptionModel == "binomialTree" {
		// 	option.OptionTheo.CalcBinomialTreeTheo(Prob, NumTimesteps)
		// 	optionBalance += option.Position * (option.OptionTheo.Theo - option.AverageCost)
		// }

		// Calculate realized pnl
		optionBalance += option.Profit
	}
	// fmt.Printf("Got option balance: %v\n", optionBalance)
	diff := optionBalance - lastOptionBalance
	algo.Market.BaseAsset.Quantity += diff
	lastOptionBalance = optionBalance
}

func calculateDifference(x float64, y float64) float64 {
	//Get percentage difference between 2 numbers
	if y == 0 {
		y = 1
	}
	return (x - y) / y
}

func (algo *Algo) getCostAverage(pricesFilled []float64, ordersFilled []float64) (float64, float64) {
	// print(len(prices), len(orders), len(timestamp_arr[0]))
	percentageFilled := sumArr(ordersFilled)
	if percentageFilled > 0 {
		normalizer := 1 / percentageFilled
		norm := mulArr(ordersFilled, normalizer)
		costAverage := sumArr(mulArrs(pricesFilled, norm))
		return costAverage, percentageFilled
	}
	return 0.0, 0.0
}

func MinMaxStats(history []models.History) (float64, float64, float64, float64, float64) {
	var maxProfit float64 = history[0].Profit
	var minProfit float64 = history[0].Profit

	var maxLeverage float64 = history[0].Leverage
	var minLeverage float64 = history[0].Leverage

	var drawdown float64 = 0.0
	var highestBalance float64 = 0.0

	for _, row := range history {
		if maxProfit < row.Profit {
			maxProfit = row.Profit
		}
		if minProfit > row.Profit {
			minProfit = row.Profit
		}

		if row.UBalance > highestBalance {
			highestBalance = row.UBalance
		}

		ddDiff := calculateDifference(row.UBalance, highestBalance)
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
	return minProfit, maxProfit, minLeverage, maxLeverage, drawdown
}

func getFilledBidOrders(prices []float64, orders []float64, price float64) ([]float64, []float64) {
	var p []float64
	var o []float64
	for i := range prices {
		if prices[i] > price {
			p = append(p, prices[i])
			o = append(o, orders[i])
		}
	}
	return p, o
}

func getFilledAskOrders(prices []float64, orders []float64, price float64) ([]float64, []float64) {
	var p []float64
	var o []float64
	for i := range prices {
		if prices[i] < price {
			p = append(p, prices[i])
			o = append(o, orders[i])
		}
	}
	return p, o
}

func LogBacktest(algo Algo) {
	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://a9266693f215611eaa2ab067000a9afa-324658220.us-east-2.elb.amazonaws.com:8086", //:8086
		Username: "tester",
		Password: "123456",
	})
	CheckIfError(err)

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

	pt, err := client.NewPoint(
		"result",
		tags,
		algo.Result,
		time.Now(),
	)
	bp.AddPoint(pt)

	client.Client.Write(influx, bp)
	// CheckIfError(err)
	influx.Close()
}

//Options backtesting functionality

func (algo *Algo) updateOptionsPositions() {
	//Aggregate positions
	for _, option := range algo.Market.OptionContracts {
		total := 0.
		avgPrice := 0.
		for i, qty := range option.BuyOrders.Quantity {
			adjPrice := AdjustForSlippage(option.BuyOrders.Price[i], "buy", .05)
			avgPrice = ((avgPrice * total) + (adjPrice * qty)) / (total + qty)
			total += qty
		}
		for i, qty := range option.SellOrders.Quantity {
			adjPrice := AdjustForSlippage(option.SellOrders.Price[i], "sell", .05)
			avgPrice = ((avgPrice * total) + (adjPrice * qty)) / (total + qty)
			total -= qty
		}
		//Fill open orders
		option.AverageCost = avgPrice
		option.Position = total
		option.BuyOrders = models.OrderArray{
			Quantity: []float64{},
			Price:    []float64{},
		}
		option.SellOrders = models.OrderArray{
			Quantity: []float64{},
			Price:    []float64{},
		}
	}
}

func generateActiveOptions(algo *Algo) []models.OptionContract {
	if ToIntTimestamp(algo.Timestamp)-LastOptionLoad < OptionLoadFreq*1000 {
		return algo.Market.Options
	}
	fmt.Printf("Generating active options with last option load %v, current timestamp %v\n", LastOptionLoad, ToIntTimestamp(algo.Timestamp))
	const numWeeklys = 3
	const numMonthlys = 5
	//TODO: these should be based on underlying price
	const minStrike = 5000.
	const maxStrike = 20000.
	//Build expirys
	var expirys []int
	currentTime := ToTimeObject(algo.Timestamp)
	for i := 0; i < numWeeklys; i++ {
		expiry := TimeToTimestamp(GetNextFriday(currentTime))
		expirys = append(expirys, expiry)
		currentTime = currentTime.Add(time.Hour * 24 * 7)
	}
	currentTime = ToTimeObject(algo.Timestamp)
	for i := 0; i < numMonthlys; i++ {
		expiry := TimeToTimestamp(GetLastFridayOfMonth(currentTime))
		if !intInSlice(expiry, expirys) {
			expirys = append(expirys, expiry)
		}
		currentTime = currentTime.Add(time.Hour * 24 * 28)
	}
	fmt.Printf("Generated expirys: %v\n", expirys)
	strikes := Arange(minStrike, maxStrike, StrikeInterval)
	fmt.Printf("Generated strikes: %v\n", strikes)
	var optionContracts []models.OptionContract
	for _, expiry := range expirys {
		for _, strike := range strikes {
			for _, optionType := range []string{"call", "put"} {
				vol := GetNearestVol(VolData, ToIntTimestamp(algo.Timestamp))
				optionTheo := models.NewOptionTheo(optionType, algo.Market.Price, strike, ToIntTimestamp(algo.Timestamp), expiry, 0, vol, -1)
				optionContract := models.OptionContract{
					Symbol:           GetDeribitOptionSymbol(expiry, strike, algo.Market.QuoteAsset.Symbol, optionType),
					Strike:           strike,
					Expiry:           expiry,
					OptionType:       optionType,
					AverageCost:      0,
					Profit:           0,
					TickSize:         TickSize,
					MakerFee:         MakerFee,
					TakerFee:         TakerFee,
					MinimumOrderSize: MinTradeAmount,
					Position:         0,
					OptionTheo:       *optionTheo,
					Status:           "open",
					MidMarketPrice:   -1.,
				}
				optionContracts = append(optionContracts, optionContract)
			}
		}
	}
	LastOptionLoad = ToIntTimestamp(algo.Timestamp)
	return optionContracts
}

func (algo *Algo) updateActiveOptions() {
	activeOptions := generateActiveOptions(algo)
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
			fmt.Printf("Found new active option: %v\n", activeOption.OptionTheo.String())
		}
	}
}

func GetNearestVol(volData []models.ImpliedVol, time int) float64 {
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

func intInSlice(a int, list []int) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}
