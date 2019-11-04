package algo

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"unsafe"

	"github.com/block26/TheAlgoV2/models"
	"github.com/block26/tantra-plot/tantraplot"
	"github.com/block26/tantra-plot/plotting"
	"github.com/block26/tantra-plot/gui"
	"github.com/block26/tantra-plot/timestamp"

	"github.com/gocarina/gocsv"
)

var history []models.History

// var minimumOrderSize = 25

func RunBacktest(a Algo, rebalance func(float64, *Algo), setupData func(*[]models.Bar, *Algo)) {
	log.Println("Loading Data... ")
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(dir + "/1m.csv")
	dataFile, err := os.OpenFile(dir+"/1m.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer dataFile.Close()
	log.Println("Done Loading Data... ")

	bars := []models.Bar{}

	if err := gocsv.UnmarshalFile(dataFile, &bars); err != nil { // Load bars from file
		panic(err)
	}

	// fmt.Println(unsafe.Sizeof(bars))
	var barSize = 0.0
	for _, bar := range bars {
		barSize = barSize + float64(unsafe.Sizeof(bar))
		// fmt.Println(int())
	}
	fmt.Printf("barSize %.2f \n", barSize)
	setupData(&bars, &a)
	score := runSingleTest(&bars, a, rebalance)
	log.Println("Score", score)
	// optimize(bars)
}


func PlotHistory(windowsize int, scrollspeed int) error {
	env := gui.Environment{nil, nil, nil, nil, nil, 0, 0, false, 0, false, 0, 0, 0}

	balance := plotting.LinePlot {nil, nil, 0, 0, 0.05, 0.05, 0.45, 0.28, 0, 0xff7700,
																 "Timestamp", "Balance", "Balance", "%2.2f", 5, "%2.2f",
																 3, 6, 2, 0, scrollspeed, windowsize}
  quantity:= plotting.LinePlot {nil, nil, 0, 0, 0.38, 0.05, 0.45, 0.28, 0, 0xff7700,
															   "Timestamp", "Quantity", "Quantity", "%2.2f", 5, "%2.2f",
															   3, 6, 2, 0, scrollspeed, windowsize}
  avgcost := plotting.MultiPlot{nil, nil, 0, 0, 0, 0.71, 0.05, 0.45, 0.28, 0,
															 	 "Timestamp", "Cost", "Average Cost", "%2.2f", 5, "%2.2f",
															 	 3, 6, 2, 0, scrollspeed, windowsize}
  leverage:= plotting.LinePlot {nil, nil, 0, 0, 0.05, 0.55, 0.45, 0.28, 0, 0xff7700,
															   "Timestamp", "Leverage", "Leverage", "%2.2f", 5, "%2.2f",
															   3, 6, 2, 0, scrollspeed, windowsize}
  profit  := plotting.LinePlot {nil, nil, 0, 0, 0.71, 0.55, 0.45, 0.28, 0, 0xff7700,
															   "Timestamp", "Profit", "Profit", "%2.2f", 5, "%2.2f",
															   3, 6, 2, 0, scrollspeed, windowsize}

	var timestamps []timestamp.Timestamp
	var balances   []float32
	var quantities []float32
	var avgcosts   []float32
	var leverages  []float32
	var profits    []float32
	var prices     []float32
	for i := 0; i < len(history); i++ {
		timestamp, err := timestamp.ParseTimeStamp(history[i].Timestamp)
		if err != nil {
			return err
		}
		timestamps = append(timestamps, timestamp)
		balances   = append(balances  , float32(history[i].Balance    ))
		quantities = append(quantities, float32(history[i].Quantity   ))
		avgcosts   = append(avgcosts  , float32(history[i].AverageCost))
		leverages  = append(leverages , float32(history[i].Leverage   ))
		profits    = append(profits   , float32(history[i].Profit     ))
		prices     = append(prices    , float32(history[i].Price      ))
	}

	env.InsertLinePlot (balance)
	env.InsertLinePlot (quantity)
	env.InsertMultiPlot(avgcost)
	env.InsertLinePlot (leverage)
	env.InsertLinePlot (profit)

	env.InsertLineData (timestamps, balances  , 0)
	env.InsertLineData (timestamps, quantities, 1)
	env.InsertLineData (timestamps, leverages , 2)
	env.InsertLineData (timestamps, profits   , 3)

	env.InsertMultiData(timestamps, []plotting.Line{plotting.Line{avgcosts, 0xff7700}, plotting.Line{prices, 0x0077ff}}, 0)

	w, err := tantraplot.MakeWindowWithEnv(&env, true)
	if err != nil {
		return err
	}

	cont := true
	for cont {
		cont = w.StepWindow()
	}

	return nil
}


func runSingleTest(data *[]models.Bar, algo Algo, rebalance func(float64, *Algo)) float64 {
	start := time.Now()
	// starting_algo.Asset.BaseBalance := 0
	timestamp := ""
	idx := 0
	log.Println("Running", len(*data), "bars")
	for _, bar := range *data {
		if timestamp == "" {
			log.Println("Start Timestamp", bar.Timestamp)
			// 	//Set average cost if starting with a quote balance
			if algo.Asset.Quantity > 0 {
				algo.Asset.AverageCost = bar.Close
			}
		}
		timestamp = bar.Timestamp
		if idx > algo.DataLength {
			algo.Index = idx
			algo.Asset.Price = bar.Open
			rebalance(bar.Open, &algo)
			//Check which buys filled
			pricesFilled, ordersFilled := getFilledBidOrders(algo.BuyOrders.Price, algo.BuyOrders.Quantity, bar.Low)
			fillCost, fillPercentage := algo.getCostAverage(pricesFilled, ordersFilled)
			algo.UpdateBalance(fillCost, algo.Asset.Buying*fillPercentage)

			//Check which sells filled
			pricesFilled, ordersFilled = getFilledAskOrders(algo.SellOrders.Price, algo.SellOrders.Quantity, bar.High)
			fillCost, fillPercentage = algo.getCostAverage(pricesFilled, ordersFilled)
			algo.UpdateBalance(fillCost, algo.Asset.Selling*-fillPercentage)

			// updateBalanceXBTStrat(bar)
			algo.logState(timestamp)
			// history.Balance[len(history.Balance)-1], == portfolio value
			// portfolioValue := history.Balance[len(history.Balance)-1]
		}
		idx++
	}

	elapsed := time.Since(start)
	log.Println("End Timestamp", timestamp)
	minProfit, maxProfit, _, maxLeverage := MinMaxStats(history)

	log.Printf("Balance %0.4f \n", history[len(history)-1].Balance)
	log.Printf("Cost %0.4f \n", history[len(history)-1].AverageCost)
	log.Printf("Quantity %0.4f \n", history[len(history)-1].Quantity)
	log.Printf("Max Leverage %0.4f \n", maxLeverage)

	log.Printf("Max Profit %0.4f \n", maxProfit)
	log.Printf("Max Drawdown %0.4f \n", minProfit)
	log.Println("Execution Speed", elapsed)
	//Very primitive score, how much leverage did I need to achieve this balance

	historyFile, err := os.OpenFile("balance.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer historyFile.Close()

	err = gocsv.MarshalFile(&history, historyFile) // Use this to save the CSV back to the file
	if err != nil {
		panic(err)
	}

	return 1 //history.Balance[len(history.Balance)-1] / (maxLeverage + 1)
}

func (algo *Algo) logState(timestamp string) {
	// history.Timestamp = append(history.Timestamp, timestamp)
	var balance float64
	if algo.Futures {
		balance = algo.Asset.BaseBalance
		algo.Asset.Leverage = (math.Abs(algo.Asset.Quantity) / algo.Asset.Price) / algo.Asset.BaseBalance
	} else {
		balance = algo.Asset.BaseBalance + (algo.Asset.Quantity * algo.Asset.Price)
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		algo.Asset.Leverage = (math.Abs(algo.Asset.Quantity)) / (algo.Asset.BaseBalance * algo.Asset.Price)
		// history.Balance = append(history.Balance, balance)
	}
	// history.Quantity = append(history.Quantity, algo.Asset.Quantity)
	// history.AverageCost = append(history.AverageCost, algo.Asset.AverageCost)

	// history.Leverage = append(history.Leverage, algo.Asset.Leverage)
	algo.Asset.Profit = algo.CurrentProfit(algo.Asset.Price) * algo.Asset.Leverage
	// history.Profit = append(history.Profit, algo.Asset.Profit)

	history = append(history, models.History{
		Timestamp:   timestamp,
		Balance:     balance,
		UBalance:    balance + (balance * algo.Asset.Profit),
		Quantity:    algo.Asset.Quantity,
		AverageCost: algo.Asset.AverageCost,
		Leverage:    algo.Asset.Leverage,
		Profit:      algo.Asset.Profit,
		Price:       algo.Asset.Price,
	})

	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Asset.BaseBalance*algo.Asset.Price+(algo.Asset.Quantity), algo.Asset.Delta, algo.Asset.BaseBalance, algo.Asset.Quantity, algo.Asset.Price, algo.Asset.AverageCost))
	}
}

func (algo *Algo) CurrentProfit(price float64) float64 {
	if algo.Asset.Quantity < 0 {
		return calculateDifference(algo.Asset.AverageCost, price)
	} else {
		return calculateDifference(price, algo.Asset.AverageCost)
	}
}

func (algo *Algo) UpdateBalance(fillCost float64, fillAmount float64) {
	// log.Printf("fillCost %.2f -> fillAmount %.2f\n", fillCost, fillCost*fillAmount)
	if math.Abs(fillAmount) > 0 {
		newQuantity := fillCost * fillAmount
		fee := math.Abs(fillAmount) * algo.Asset.MakerFee
		// log.Printf("fillCost %.2f -> fillAmount %.2f -> Fee %.2f \n", fillCost, fillCost*fillAmount, fee)
		currentCost := (algo.Asset.Quantity * algo.Asset.AverageCost)
		totalQuantity := algo.Asset.Quantity + newQuantity
		newCost := fillCost * newQuantity
		if algo.Futures {
			if (newQuantity >= 0 && algo.Asset.Quantity >= 0) || (newQuantity <= 0 && algo.Asset.Quantity <= 0) {
				//Adding to position
				algo.Asset.AverageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else {
				var diff float64
				if fillAmount > 0 {
					diff = calculateDifference(algo.Asset.AverageCost, fillCost)
				} else {
					diff = calculateDifference(fillCost, algo.Asset.AverageCost)
				}
				algo.Asset.BaseBalance = algo.Asset.BaseBalance + ((math.Abs(newQuantity) * diff) / fillCost)
			}
			algo.Asset.Quantity = algo.Asset.Quantity + newQuantity
		} else {
			if newQuantity >= 0 {
				//Adding to position
				algo.Asset.AverageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			}
			// else {
			// 	var diff float64
			// 	if fillAmount > 0 {
			// 		diff = calculateDifference(algo.Asset.AverageCost, fillCost)
			// 	} else {
			// 		diff = calculateDifference(fillCost, algo.Asset.AverageCost)
			// 	}
			// 	algo.Asset.BaseBalance = algo.Asset.BaseBalance + ((math.Abs(newQuantity)*diff) / fillCost)
			// }
			algo.Asset.BaseBalance = algo.Asset.BaseBalance - newCost
			algo.Asset.Quantity = algo.Asset.Quantity + newQuantity
		}
		algo.Asset.BaseBalance = algo.Asset.BaseBalance - fee
	}
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

func MinMaxStats(history []models.History) (float64, float64, float64, float64) {
	var maxProfit float64 = history[0].Profit
	var minProfit float64 = history[0].Profit

	var maxLeverage float64 = history[0].Leverage
	var minLeverage float64 = history[0].Leverage
	for _, row := range history {
		if maxProfit < row.Profit {
			maxProfit = row.Profit
		}
		if minProfit > row.Profit {
			minProfit = row.Profit
		}

		if maxLeverage < row.Leverage {
			maxLeverage = row.Leverage
		}
		if minLeverage > row.Leverage {
			minLeverage = row.Leverage
		}
	}
	return minProfit, maxProfit, minLeverage, maxLeverage
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
