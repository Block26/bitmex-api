package main

import (
    "fmt"
	"os"
	"math"
	"time"

	"github.com/gocarina/gocsv"
)

type Bar struct {
    Timestamp string    `csv:"timestamp"`
    Open 	  float64   `csv:"open"`
    High      float64   `csv:"high"`
    Low       float64   `csv:"low"`
    Close     float64   `csv:"close"`
}

type Portfolio struct {
    BaseAsset  Asset
    QuoteAsset Asset
}

type Asset struct {
    Quantity    float64
    Price       float64
}

func main() {
	fmt.Println("Loading Data... ")
	dataFile, err := os.OpenFile("./1m.csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer dataFile.Close()

	bars := []*Bar{}

	if err := gocsv.UnmarshalFile(dataFile, &bars); err != nil { // Load bars from file
		panic(err)
	}
	runBacktest(bars)
}

var baseBalance = 3500.0
var quoteBalance = 1.0
var portfolioValue = 0.0
var averageCost = 0.0

func runBacktest(data []*Bar) {
	start := time.Now()
	// starting_quoteBalance := 1
	// starting_baseBalance := 0
	liquidPerBar := 0.01 //Defined as btc but will be % in the future
	index := ""
	for _, bar := range data {
		if index == "" {
			fmt.Println("Start Timestamp", bar.Timestamp)
			//Set average cost if starting with a quote balance
			averageCost = bar.Close
		}
		index = bar.Timestamp

		var priceArr, orderArr []float64
		var selling float64

		// Delta Target Strategy
		// targetDelta := 1.0
		// pValue := quoteBalance * averageCost
		// delta := pValue / (baseBalance + pValue)

		// priceArr, orderArr = createSpread(1, 2, bar.Open, 0.01, 0.5, 20)
		// buying := liquidPerBar
		// if delta < targetDelta { buying = buying + (targetDelta - delta) }
		// pricesFilled, ordersFilled := getFilledBidOrders(priceArr, orderArr, bar.Low)
		// fillCost, fillPercentage := getCostAverage(pricesFilled, ordersFilled)
		// rebalance(fillCost, buying * fillPercentage)
		
		// priceArr, orderArr = createSpread(-1, 2, bar.Open, 0.01, 0.5, 20)
		// selling = liquidPerBar
		// if delta > targetDelta { selling = selling + (delta - targetDelta) }
		// pricesFilled, ordersFilled = getFilledAskOrders(priceArr, orderArr, bar.High)
		// fillCost, fillPercentage = getCostAverage(pricesFilled, ordersFilled)
		// rebalance(fillCost, selling * -fillPercentage)

		// Buys
		buying := liquidPerBar
		// if baseBalance < 0 { buying = buying + (math.Abs(baseBalance) / bar.Open)  }
		if baseBalance < 0 { 
			buying = buying //+ (math.Abs(baseBalance) / bar.Open)
			startBuyPrice := bar.Open
			// if averageCost < bar.Open {
			// 	startBuyPrice = averageCost
			// }
			priceArr, orderArr = createSpread(1, 1, startBuyPrice, 0.01, 0.5, 20)
		} else {
			priceArr, orderArr = createSpread(1, 2, bar.Open, 0.01, 0.5, 20)
		}
		pricesFilled, ordersFilled := getFilledBidOrders(priceArr, orderArr, bar.Low)
		fillCost, fillPercentage := getCostAverage(pricesFilled, ordersFilled)
		// rebalance(fillCost, buying * fillPercentage)

		// Sells
		selling = liquidPerBar
		if baseBalance > 0 { 
			selling = selling //+ (math.Abs(baseBalance) / bar.Open)
			startSellPrice := bar.Open
			// if averageCost > bar.Open {
			// 	startSellPrice = averageCost
			// }
			priceArr, orderArr = createSpread(-1, 1, startSellPrice, 0.01, 0.5, 20)
		} else {
			priceArr, orderArr = createSpread(-1, 2, bar.Open, 0.01, 0.5, 20)
		}
		pricesFilled, ordersFilled = getFilledAskOrders(priceArr, orderArr, bar.High)
		fillCost, fillPercentage = getCostAverage(pricesFilled, ordersFilled)
		rebalance(fillCost, selling * -fillPercentage)

		portfolioValue = (baseBalance + (quoteBalance * bar.Close)) / bar.Close
	}

	elapsed := time.Since(start)
	fmt.Println("End Timestamp", index, "Cost", averageCost, "Quantity", quoteBalance)
	fmt.Println("Execution Speed", elapsed)
}

func rebalance(fillCost float64, fillAmount float64) {
	newCost := fillCost * fillAmount
	if math.Abs(fillAmount) > 0 && (quoteBalance + fillAmount > 0) {
		// if baseBalance + fillCost > 0 {
			//I can successfully complete this order
			// base_cost := fillAmount / fillCost
		
		// fmt.Println("fillAmount", fillAmount, "fillCost", fillCost)
		// currentCost := (quoteBalance * cost)
		currentCost := (quoteBalance * averageCost)
		fmt.Printf("currentCost %.2f -> fill_cost %.2f\n", currentCost, fillCost)

		baseBalance = baseBalance - newCost
		quoteBalance = quoteBalance + fillAmount

		// if newCost > 0 && baseBalance > 0 {
		// 	averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(quoteBalance)
		// } else 
		if newCost < 0 && baseBalance < 0  {
			averageCost = (math.Abs(newCost) + math.Abs(currentCost)) / math.Abs(quoteBalance)
		}
		// if (fillAmount > 0 && quoteBalance >= 0) || (fillAmount < 0 && quoteBalance <= 0) {
		// 	// Need to update my cost average
		// 	currentCost := (quoteBalance * cost)
		// 	if cost <= 0 {
		// 		cost = newCost
		// 	} else {
		// 		cost = (math.Abs(newCost) + math.Abs(currentCost))// / math.Abs(quoteBalance)
		// 	}
		// }
		// fmt.Printf("fillCost %.2f -> Bought %.2f\n", fillCost, fillCost * fillAmount)
		fmt.Printf("portfolioValue %.4f quoteBalance %.2f baseBalance %.2f averageCost %0.2f\n", portfolioValue, quoteBalance, baseBalance, averageCost)
		// } else {
		// 	fmt.Println("ERRROOOROROROR")
		// 	return
		// }
	}
}

func getCostAverage(pricesFilled []float64, ordersFilled []float64) (float64, float64) {
	// print(len(prices), len(orders), len(index_arr[0]))
	percentageFilled := sumArr(ordersFilled)
	if percentageFilled > 0 {
		normalizer := 1/percentageFilled
		norm := mulArr(ordersFilled, normalizer)
		costAverage := mulArrs(pricesFilled, norm)
		return sumArr(costAverage), percentageFilled
	} else {
		return 0.0, 0.0
	}
}

func getFilledBidOrders(prices []float64, orders []float64, price float64) ([]float64, []float64) {
	var p []float64
	var o []float64
	for i := range prices {
		if prices[i] >= price {
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
		if prices[i] <= price {
			p = append(p, prices[i])
			o = append(o, orders[i])
		}
    }
    return p, o
}

func createSpread(weight int32, confidence float64, price float64, spread float64, tick_size float64, max_orders float64) ([]float64, []float64) {
	x_start := 0.0
	if weight == 1 {
		x_start = price - (price*spread)
	} else {
		x_start = price
	}

	x_end := x_start + (x_start*spread)
	diff := x_end - x_start

	if diff / tick_size >= max_orders {
		tick_size = diff / (max_orders-1)
	}

	price_arr := arange(x_start, x_end, tick_size)
	temp := divArr(price_arr, x_start)
	// temp := (price_arr/x_start)-1

	dist := expArr(temp, confidence)

	normalizer := 1/sumArr(dist)
	order_arr := mulArr(dist, normalizer)
	if weight == 1 { 
		order_arr = reverseArr(order_arr)
	}

	return price_arr, order_arr
}

func reverseArr(a []float64) []float64 {
	for i := len(a)/2-1; i >= 0; i-- {
		opp := len(a)-1-i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func arange(min float64, max float64, step float64) []float64 {
    a := make([]float64, int32((max-min)/step)+1)
    for i := range a {
        a[i] = min + (float64(i) * step)
    }
    return a
}

func expArr(arr []float64, exp float64) []float64 {
    a := make([]float64, len(arr))
    for i := range arr {
        a[i] = a[i] + exponent(arr[i], exp)-1
    }
    return a
}

func mulArrs(a []float64, b []float64) []float64 {
    n := make([]float64, len(a))
    for i := range a {
        n[i] = a[i] * b[i]
    }
    return n
}

func mulArr(arr []float64, multiple float64) []float64 {
    a := make([]float64, len(arr))
    for i := range arr {
        a[i] = float64(arr[i]) * multiple
    }
    return a
}

func divArr(arr []float64, divisor float64) []float64 {
    a := make([]float64, len(arr))
    for i := range arr {
        a[i] = float64(arr[i]) / divisor
    }
    return a
}

func sumArr(arr []float64) float64 {
    sum := 0.0
    for i := range arr {
        sum = sum + arr[i]
    }
    return sum
}

func exponent(x, y float64 ) float64 {
	return math.Pow(x, y)
}