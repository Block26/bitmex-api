package main

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"encoding/json"
	"unsafe"

	"github.com/block26/TheAlgoV2/models"

	"github.com/gocarina/gocsv"
)

var history models.History
var algo models.MMConfig
var fee = 0.0
var minTickSize = 0.00001
var futures = false
var debug = true

// var minimumOrderSize = 25

func runBacktest() {
	log.Println("Loading Data... ")
	dataFile, err := os.OpenFile("./1m-dcr.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer dataFile.Close()
	log.Println("Done Loading Data... ")

	bars := []*models.Bar{}

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

	// {"BaseBalance":133.19306310441956,"Quantity":150586.2208447658,"AverageCost":10416.870228463727,"MaxOrders":20,"EntrySpread":0.007091541970168746,"EntryConfidence":2.8463638315235507,"ExitSpread":0.028536544770263263,"ExitConfidence":0.5240603560083594,"Liquidity":0.21034473882199728}

	// XBTUSD
	// algo := models.MMConfig{
	// 	BaseBalance: 1.0,
	// 	Quantity: 0.0,
	// 	AverageCost: 0.0,
	// 	MaxOrders: 15.0,
	// 	EntrySpread: 0.02,
	// 	EntryConfidence: 2,
	// 	ExitSpread: 0.02,
	// 	ExitConfidence: 0.1,
	// 	Liquidity: 0.03,
	// 	MaxLeverage: 0.2,
	// }

	//DCRBTC
	algo := models.MMConfig{
		BaseBalance:     1.0,
		Quantity:        220.0,
		AverageCost:     0.0,
		MaxOrders:       15,
		EntrySpread:     0.05,
		EntryConfidence: 1,
		ExitSpread:      0.03,
		ExitConfidence:  0.1,
		Liquidity:       0.1,
		MaxLeverage:     0.2,
	}
	// entrySpread=0.005012, exitSpread=0.029661, entryConfidence=1.610416, exitConfidence=0.444074, liquidity=0.249863

	// algo := MMConfig{BaseBalance:1,Quantity:0,AverageCost:0, MaxOrders:20,
	// 	EntrySpread:0.005012,EntryConfidence:1.610416,
	// 	ExitSpread:0.029661,ExitConfidence:0.444074,
	// 	Liquidity:0.249863}

	score := runSingleTest(bars, algo)
	log.Println("Score", score)
	// optimize(bars)

}

func print(index string, msg string) {
	if debug {
		fmt.Println(index, msg)
	}
}

func runSingleTest(data []*models.Bar, algoConfig models.MMConfig) float64 {
	start := time.Now()
	algo = algoConfig
	// starting_algo.BaseBalance := 0
	index := ""
	log.Println("Running", len(data), "bars")
	for _, bar := range data {
		if index == "" {
			log.Println("Start Timestamp", bar.Timestamp)
			// 	//Set average cost if starting with a quote balance
			if algo.Quantity > 0 {
				algo.AverageCost = bar.Close
			}
		}
		index = bar.Timestamp

		liquidPerBar := algo.Liquidity * algo.BaseBalance
		// Delta Target Strategy
		targetDelta := 0.6
		pValue := algo.Quantity * algo.AverageCost
		delta := pValue / (algo.BaseBalance + pValue)

		buyOrders := createSpread(1, algo.EntryConfidence, bar.Open, algo.EntrySpread, minTickSize, algo.MaxOrders)
		buying := liquidPerBar
		if delta < targetDelta {
			buying = buying + (targetDelta - delta)
		}
		// print(index, fmt.Sprintf("buying %0.4f", buying))
		buying = buying / bar.Open
		pricesFilled, ordersFilled := getFilledBidOrders(buyOrders.Price, buyOrders.Quantity, bar.Low)
		fillCost, fillPercentage := getCostAverage(pricesFilled, ordersFilled)
		updateBalance(fillCost, buying*fillPercentage)

		sellOrders := createSpread(-1, algo.EntryConfidence, bar.Open, algo.EntrySpread, minTickSize, algo.MaxOrders)
		selling := liquidPerBar
		if delta > targetDelta {
			selling = selling + (delta - targetDelta)
		}
		// print(index, fmt.Sprintf("selling %0.4f", selling))
		selling = selling / bar.Open
		pricesFilled, ordersFilled = getFilledAskOrders(sellOrders.Price, sellOrders.Quantity, bar.High)
		fillCost, fillPercentage = getCostAverage(pricesFilled, ordersFilled)
		updateBalance(fillCost, selling*-fillPercentage)

		// updateBalanceXBTStrat(bar)
		logState(bar.Open)
		// history.Balance[len(history.Balance)-1], == portfolio value
		portfolioValue := history.Balance[len(history.Balance)-1]
		print(index, fmt.Sprintf("Balance %.2f | Delta %0.2f | BTC %0.2f | DCR %.2f | Price %.5f - Cost %.5f", portfolioValue, delta, algo.BaseBalance, algo.Quantity, bar.Open, algo.AverageCost))
	}

	elapsed := time.Since(start)
	log.Println("End Timestamp", index)
	min, max := MinMax(history.Quantity)
	_, maxLeverage := MinMax(history.Leverage)

	log.Printf("Balance %0.4f \n", history.Balance[len(history.Balance)-1])
	log.Printf("Cost %0.4f \n", history.AverageCost[len(history.AverageCost)-1])
	log.Printf("Quantity %0.4f \n", history.Quantity[len(history.Quantity)-1])
	log.Printf("Max Long Exposure %0.4f \n", max)
	log.Printf("Max Short Exposure %0.4f \n", min)
	log.Printf("Max Leverage %0.4f \n", maxLeverage)

	drawdown, maxProfit := MinMax(history.Profit)
	log.Printf("Max Profit %0.4f \n", maxProfit)
	log.Printf("Max Drawdown %0.4f \n", drawdown)

	log.Println("Execution Speed", elapsed)
	config, _ := json.Marshal(algo)
	log.Println(string(config))
	//Very primitive score, how much leverage did I need to achieve this balance
	return history.Balance[len(history.Balance)-1] / (maxLeverage + 1)
}

func logState(price float64) {
	if futures {
		history.Balance = append(history.Balance, algo.BaseBalance)
	} else {
		balance := algo.BaseBalance + (algo.Quantity * price)
		history.Balance = append(history.Balance, balance)
	}
	history.Quantity = append(history.Quantity, algo.Quantity)
	history.AverageCost = append(history.AverageCost, algo.AverageCost)

	leverage := (math.Abs(algo.Quantity) / price) / algo.BaseBalance
	history.Leverage = append(history.Leverage, leverage)
	algo.Profit = currentProfit(price) * leverage
	history.Profit = append(history.Profit, algo.Profit)
}

func currentProfit(price float64) float64 {
	if algo.Quantity < 0 {
		return calculateDifference(algo.AverageCost, price)
	} else {
		return calculateDifference(price, algo.AverageCost)
	}
}

func updateBalance(fillCost float64, fillAmount float64) {
	if math.Abs(fillAmount) > 0 {
		newQuantity := fillCost * fillAmount
		// log.Printf("fillCost %.2f -> fillAmount %.2f\n", fillCost, fillCost*fillAmount)
		currentCost := (algo.Quantity * algo.AverageCost)
		totalQuantity := algo.Quantity + newQuantity
		newCost := fillCost * newQuantity
		if futures {
			if (newQuantity >= 0 && algo.Quantity >= 0) || (newQuantity <= 0 && algo.Quantity <= 0) {
				//Adding to position
				algo.AverageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			} else {
				var diff float64
				if fillAmount > 0 {
					diff = calculateDifference(algo.AverageCost, fillCost)
				} else {
					diff = calculateDifference(fillCost, algo.AverageCost)
				}
				algo.BaseBalance = algo.BaseBalance + ((math.Abs(newQuantity) * diff) / fillCost)
			}
			algo.Quantity = algo.Quantity + newQuantity
		} else {
			if newQuantity >= 0 {
				//Adding to position
				algo.AverageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(totalQuantity)
			}
			// else {
			// 	var diff float64
			// 	if fillAmount > 0 {
			// 		diff = calculateDifference(algo.AverageCost, fillCost)
			// 	} else {
			// 		diff = calculateDifference(fillCost, algo.AverageCost)
			// 	}
			// 	algo.BaseBalance = algo.BaseBalance + ((math.Abs(newQuantity)*diff) / fillCost)
			// }
			algo.BaseBalance = algo.BaseBalance - newCost
			algo.Quantity = algo.Quantity + newQuantity
		}
	}
}

func calculateDifference(x float64, y float64) float64 {
	//Get percentage difference between 2 numbers
	if y == 0 {
		y = 1
	}
	return (x - y) / y
}

func getCostAverage(pricesFilled []float64, ordersFilled []float64) (float64, float64) {
	// print(len(prices), len(orders), len(index_arr[0]))
	percentageFilled := sumArr(ordersFilled)
	if percentageFilled > 0 {
		normalizer := 1 / percentageFilled
		norm := mulArr(ordersFilled, normalizer)
		costAverage := sumArr(mulArrs(pricesFilled, norm))
		costAverage = costAverage - (costAverage * fee)
		return costAverage, percentageFilled
	}
	return 0.0, 0.0
}

func MinMax(array []float64) (float64, float64) {
	var max float64 = array[0]
	var min float64 = array[0]
	for _, value := range array {
		if max < value {
			max = value
		}
		if min > value {
			min = value
		}
	}
	return min, max
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
