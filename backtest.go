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
	"github.com/tantralabs/yantra/models"
	// "github.com/tantralabs/yantra/options"
	"github.com/tantralabs/yantra/tantradb"
	. "gopkg.in/src-d/go-git.v4/_examples"
)

// var MinimumOrderSize = 25
var currentRunUUID time.Time

var VolData []models.ImpliedVol
var lastOptionBalance = 0.
var LastOptionLoad = 0
var OptionLoadFreq = 15

//TODO: these should be a config somewhere
const StrikeInterval = 250.
const TickSize = .1
const MinTradeAmount = .1
const MakerFee = 0.
const TakerFee = .001

func RunBacktest(data []*models.Bar, algo Algo, rebalance func(Algo) Algo, setupData func([]*models.Bar, Algo)) Algo {
	setupData(data, algo)
	start := time.Now()
	var history []models.History
	var timestamp time.Time
	// starting_algo.Market.BaseBalance := 0
	dataStart := int(data[0].Timestamp)
	if algo.Market.Options {
		volEnd := int(data[len(data)-1].Timestamp)
		fmt.Printf("Vol data start: %v, end %v\n", dataStart, volEnd)
		VolData = tantradb.LoadImpliedVols("XBTUSD", dataStart, volEnd)
		algo.Market.OptionContracts = generateActiveOptions(&algo)
		fmt.Printf("Len vol data: %v\n", len(VolData))
	}
	algo.Timestamp = TimestampToTime(dataStart).String()
	idx := 0
	log.Println("Running", len(data), "bars")
	for _, bar := range data {
		if idx == 0 {
			log.Println("Start Timestamp", time.Unix(bar.Timestamp/1000, 0))
			fmt.Printf("Running backtest with quote asset quantity %v and base asset quantity %v, fill type %v\n", algo.Market.QuoteAsset.Quantity, algo.Market.BaseAsset.Quantity, algo.FillType)
			// 	//Set average cost if starting with a quote balance
			if algo.Market.QuoteAsset.Quantity > 0 {
				algo.Market.AverageCost = bar.Close
			}
		}
		timestamp = time.Unix(bar.Timestamp/1000, 0)
		if idx > algo.DataLength+1 {
			algo.Index = idx
			algo.Market.Price = *bar
			// algo.updateActiveOptions()
			algo = rebalance(algo)
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
			} else if algo.FillType == "close" {
				algo.updateBalanceFromFill(bar.Close)
			} else if algo.FillType == "open" {
				algo.updateBalanceFromFill(bar.Open)
			}
			// fmt.Printf("Updated balances: quote asset %v, base asset %v\n", algo.Market.QuoteAsset.Quantity, algo.Market.BaseAsset.Quantity)
			// updateBalanceXBTStrat(bar)
			if algo.Market.Options {
				algo.updateOptionsPositions()
			}
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
	if algo.DecisionInterval == "1h" {
		score = score * math.Sqrt(365*24)
	} else if algo.DecisionInterval == "1m" {
		score = score * math.Sqrt(365*24*60)
	}

	if math.IsNaN(score) {
		score = -100
	}

	if history[historyLength-1].Balance < 0 {
		score = -100
	}

	// fmt.Printf("Last option balance: %v\n", lastOptionBalance)
	algo.Params["EntryOrderSize"] = algo.EntryOrderSize
	algo.Params["ExitOrderSize"] = algo.ExitOrderSize
	algo.Params["DeleverageOrderSize"] = algo.DeleverageOrderSize

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
	// fmt.Printf("Updating balance with fill cost %v, fill amount %v, qaq %v, baq %v\n", fillCost, fillAmount, algo.Market.QuoteAsset.Quantity, algo.Market.BaseAsset.Quantity)
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
				// fmt.Printf("Adding to position: avg cost %v\n", algo.Market.AverageCost)
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
				// fmt.Printf("Updating portion fill qty with baq %v, portion fill qty %v, diff %v, avg cost %v\n", algo.Market.BaseAsset.Quantity, portionFillQuantity, diff, algo.Market.AverageCost)
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
				// fmt.Printf("Updating full fill quantity with baq %v, newQuantity %v, diff %v, avg cost %v\n", algo.Market.BaseAsset.Quantity, newQuantity, diff, algo.Market.AverageCost)
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
	if algo.Market.Options {
		algo.updateOptionBalance()
	}
}

func (algo *Algo) updateOptionBalance() {
	// fmt.Printf("Updating option balance...\n")
	optionBalance := 0.
	for _, option := range algo.Market.OptionContracts {
		// Calculate unrealized pnl
		if math.Abs(option.Position) > 0 {
			option.OptionTheo.UnderlyingPrice = algo.Market.Price.Close
			option.OptionTheo.CalcBlackScholesTheo(false)
			// fmt.Printf("Updating balance for %v with position %v theo %v avg cost %v\n", option.Symbol, option.Position, option.OptionTheo.Theo, option.AverageCost)
			optionBalance += option.Position * (option.OptionTheo.Theo - option.AverageCost)
			// Calculate realized pnl
			optionBalance += option.Profit
		}
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
	fmt.Printf("Updating options positions...\n")
	for i := range algo.Market.OptionContracts {
		option := &algo.Market.OptionContracts[i]
		total := 0.
		avgPrice := 0.
		hasAmount := false
		if len(option.SellOrders.Quantity) > 0 {
			fmt.Printf("Found orders for option %v: %v\n", option.Symbol, option.SellOrders)
		}
		for i, qty := range option.BuyOrders.Quantity {
			price := option.BuyOrders.Price[i]
			var adjPrice float64
			if price > 0 {
				// Limit order
				adjPrice = AdjustForSlippage(price, "buy", .05)
			} else {
				// Market order
				if option.OptionTheo.Theo < 0 {
					option.OptionTheo.CalcBlackScholesTheo(false)
				}
				adjPrice = AdjustForSlippage(option.OptionTheo.Theo, "buy", .05)
			}
			adjPrice = RoundToNearest(adjPrice, option.TickSize)
			if adjPrice > 0 {
				fmt.Printf("Updating avgprice with avgprice %v total %v adjprice %v qty %v\n", avgPrice, total, adjPrice, qty)
				avgPrice = ((avgPrice * total) + (adjPrice * qty)) / (total + qty)
				total += qty
			} else {
				fmt.Printf("Cannot buy option %v for adjPrice 0\n", option.Symbol)
			}
			hasAmount = true
		}
		for i, qty := range option.SellOrders.Quantity {
			price := option.SellOrders.Price[i]
			var adjPrice float64
			if price > 0 {
				// Limit order
				adjPrice = AdjustForSlippage(price, "sell", .05)
			} else {
				// Market order
				if option.OptionTheo.Theo < 0 {
					option.OptionTheo.CalcBlackScholesTheo(false)
				}
				adjPrice = AdjustForSlippage(option.OptionTheo.Theo, "sell", .05)
			}
			adjPrice = RoundToNearest(adjPrice, option.TickSize)
			if adjPrice > 0 {
				fmt.Printf("Updating avgprice with avgprice %v total %v adjprice %v qty %v\n", avgPrice, total, adjPrice, qty)
				avgPrice = math.Abs(((avgPrice * total) + (adjPrice * qty)) / (total - qty))
				total -= qty
			} else {
				fmt.Printf("Cannot sell option %v for adjPrice 0\n", option.Symbol)
			}
			hasAmount = true
		}
		if hasAmount {
			//Fill open orders
			fmt.Printf("Calcing new avg cost with avg cost %v, position %v, avgprice %v, total %v\n", option.AverageCost, option.Position, avgPrice, total)
			option.AverageCost = ((option.AverageCost * option.Position) + (avgPrice * total)) / (option.Position + total)
			option.Position += total
			option.BuyOrders = models.OrderArray{
				Quantity: []float64{},
				Price:    []float64{},
			}
			option.SellOrders = models.OrderArray{
				Quantity: []float64{},
				Price:    []float64{},
			}
			fmt.Printf("[%v] updated avgcost %v and position %v\n", option.Symbol, option.AverageCost, option.Position)
		}
	}
}

func generateActiveOptions(algo *Algo) []models.OptionContract {
	if ToIntTimestamp(algo.Timestamp)-LastOptionLoad < OptionLoadFreq*1000 {
		return algo.Market.OptionContracts
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
				optionTheo := models.NewOptionTheo(optionType, algo.Market.Price.Close, strike, ToIntTimestamp(algo.Timestamp), expiry, 0, vol, -1)
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
