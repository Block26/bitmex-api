package algo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/fatih/structs"
	"github.com/gocarina/gocsv"
	client "github.com/influxdata/influxdb1-client/v2"
	algoModels "github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/TheAlgoV2/settings"
	"github.com/tantralabs/exchanges/models"

	. "gopkg.in/src-d/go-git.v4/_examples"
)

// Load a config file containing sensitive information from a local
// json file or from an amazon secrets file
func loadConfiguration(file string, secret bool) settings.Config {
	var config settings.Config
	if secret {
		secret := getSecret(file)
		config = settings.Config{}
		json.Unmarshal([]byte(secret), &config)
		return config
	} else {
		configFile, err := os.Open(file)
		defer configFile.Close()
		if err != nil {
			log.Println(err.Error())
		}
		jsonParser := json.NewDecoder(configFile)
		jsonParser.Decode(&config)
		return config
	}
}

func LoadBars(csvFile string) []algoModels.Bar {
	var bars []algoModels.Bar

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(dir + "/" + csvFile + ".csv")
	dataFile, err := os.OpenFile(dir+"/"+csvFile+".csv", os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		panic(err)
	}
	defer dataFile.Close()
	// log.Println("Done Loading Data... ")

	if err := gocsv.UnmarshalFile(dataFile, &bars); err != nil { // Load bars from file
		panic(err)
	}

	return bars
}

//Set the liquidity available for to buy/sell. IE put 5% of my portfolio on the bid.
func (a *Algo) SetLiquidity(percentage float64, side string) float64 {
	if a.Market.Futures {
		return percentage * a.Market.BaseAsset.Quantity
	} else {
		if side == "buy" {
			return percentage * a.Market.QuoteAsset.Quantity
		}
		return percentage * ((a.Market.BaseAsset.Quantity * a.Market.Price) + a.Market.QuoteAsset.Quantity)
	}
}

//Log the state of the algo and update variables like leverage
func (algo *Algo) logState(timestamp ...string) {
	// algo.History.Timestamp = append(algo.History.Timestamp, timestamp)
	var balance float64
	if algo.Market.Futures {
		balance = algo.Market.BaseAsset.Quantity
		algo.Market.Leverage = (math.Abs(algo.Market.QuoteAsset.Quantity) / algo.Market.Price) / algo.Market.BaseAsset.Quantity
	} else {
		balance = algo.Market.BaseAsset.Quantity + (algo.Market.QuoteAsset.Quantity * algo.Market.Price)
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		algo.Market.Leverage = (math.Abs(algo.Market.QuoteAsset.Quantity)) / (algo.Market.BaseAsset.Quantity * algo.Market.Price)
	}

	algo.Market.Profit = algo.Market.BaseAsset.Quantity * (algo.CurrentProfit(algo.Market.Price) * algo.Market.Leverage)

	if timestamp != nil {
		history := algoModels.History{
			Timestamp:   timestamp[0],
			Balance:     balance,
			Quantity:    algo.Market.QuoteAsset.Quantity,
			AverageCost: algo.Market.AverageCost,
			Leverage:    algo.Market.Leverage,
			Profit:      algo.Market.Profit,
			Price:       algo.Market.Price,
		}

		if algo.Market.Futures {
			history.UBalance = balance + (balance * algo.Market.Profit)
		} else {
			history.UBalance = (algo.Market.BaseAsset.Quantity * algo.Market.Price) + algo.Market.QuoteAsset.Quantity
		}

		algo.History = append(algo.History, history)
	} else {
		algo.LogLiveState()
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Market.BaseAsset.Quantity*algo.Market.Price+(algo.Market.QuoteAsset.Quantity), 0, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.Price, algo.Market.AverageCost))
	}
}

//Log the state of the algo to influx db
func (algo *Algo) LogLiveState() {
	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://ec2-54-219-145-3.us-west-1.compute.amazonaws.com:8086",
		Username: "russell",
		Password: "KNW(12nAS921D",
	})
	CheckIfError(err)

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": algo.Name, "commit_hash": commitHash}

	fields := structs.Map(algo.Market)

	pt, err := client.NewPoint(
		"market",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	for index := 0; index < len(algo.Market.BuyOrders.Quantity); index++ {

		fields = map[string]interface{}{
			fmt.Sprintf("%0.2f", algo.Market.BuyOrders.Price[index]): algo.Market.BuyOrders.Quantity[index],
		}

		pt, err = client.NewPoint(
			"buy_orders",
			tags,
			fields,
			time.Now(),
		)
		bp.AddPoint(pt)
	}

	for index := 0; index < len(algo.Market.SellOrders.Quantity); index++ {
		fields = map[string]interface{}{
			fmt.Sprintf("%0.2f", algo.Market.SellOrders.Price[index]): algo.Market.SellOrders.Quantity[index],
		}
		pt, err = client.NewPoint(
			"sell_orders",
			tags,
			fields,
			time.Now(),
		)
		bp.AddPoint(pt)
	}

	if algo.State != nil {
		pt, err := client.NewPoint(
			"state",
			tags,
			algo.State,
			time.Now(),
		)
		CheckIfError(err)
		bp.AddPoint(pt)
	}

	err = client.Client.Write(influx, bp)
	CheckIfError(err)
	influx.Close()
}

//Create a Spread on the bid/ask, this fuction is used to create an arrary of orders that spreads across the order book.
func CreateSpread(weight int32, confidence float64, price float64, spread float64, tickSize float64, maxOrders int32) models.OrderArray {
	xStart := 0.0
	if weight == 1 {
		xStart = price - (price * spread)
	} else {
		xStart = price
	}
	xStart = Round(xStart, tickSize)

	xEnd := xStart + (xStart * spread)
	xEnd = Round(xEnd, tickSize)

	diff := xEnd - xStart

	if diff/tickSize >= float64(maxOrders) {
		newTickSize := diff / (float64(maxOrders) - 1)
		tickSize = Round(newTickSize, tickSize)
	}

	var priceArr []float64

	if weight == 1 {
		priceArr = arange(xStart, xEnd-float64(tickSize), float64(tickSize))
	} else {
		priceArr = arange(xStart, xEnd, float64(tickSize))
	}

	temp := divArr(priceArr, xStart)

	dist := expArr(temp, confidence)
	normalizer := 1 / sumArr(dist)
	orderArr := mulArr(dist, normalizer)
	if weight == 1 {
		orderArr = reverseArr(orderArr)
	}

	return models.OrderArray{Price: priceArr, Quantity: orderArr}
}

// Break down the bars into open, high, low, close arrays that are easier to manipulate.
func GetOHLCBars(bars []algoModels.Bar) ([]float64, []float64, []float64, []float64) {
	open := make([]float64, len(bars))
	high := make([]float64, len(bars))
	low := make([]float64, len(bars))
	close := make([]float64, len(bars))
	for i := range bars {
		open[i] = bars[i].Open
		high[i] = bars[i].High
		low[i] = bars[i].Low
		close[i] = bars[i].Close
	}
	return open, high, low, close
}

func Round(x, unit float64) float64 {
	return math.Round(x/unit) * unit
}

func reverseArr(a []float64) []float64 {
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func arange(min float64, max float64, step float64) []float64 {
	a := make([]float64, int32((max-min)/step)+1)
	for i := range a {
		a[i] = float64(min+step) + (float64(i) * step)
	}
	return a
}

// Apply an exponent to a slice
func expArr(arr []float64, exp float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		if arr[i] > 1 {
			a[i] = exponent(arr[i]-1, exp)
		} else {
			a[i] = exponent(arr[i], exp) - 1
		}
	}
	return a
}

// Multiply a slice by another slice of the same length
func mulArrs(a []float64, b []float64) []float64 {
	n := make([]float64, len(a))
	for i := range a {
		n[i] = a[i] * b[i]
	}
	return n
}

// Multiply a slice by a float
func mulArr(arr []float64, multiple float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = float64(arr[i]) * multiple
	}
	return a
}

// Divide all elements of a slice by a float
func divArr(arr []float64, divisor float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = float64(arr[i]) / divisor
	}
	return a
}

// Get the sum of all elements in a slice
func sumArr(arr []float64) float64 {
	sum := 0.0
	for i := range arr {
		sum = sum + arr[i]
	}
	return sum
}

func exponent(x, y float64) float64 {
	return math.Pow(x, y)
}

func createKeyValuePairs(m map[string]interface{}) string {
	b := new(bytes.Buffer)
	fmt.Fprint(b, "\n{\n")
	for key, value := range m {
		fmt.Fprint(b, " ", key, ": ", value, ",\n")
	}
	fmt.Fprint(b, "}\n")
	return b.String()
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}

func toFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}
