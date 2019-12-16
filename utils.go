package algo

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/gocarina/gocsv"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/TheAlgoV2/settings"
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

func LoadBars(csvFile string) []*models.Bar {
	var bars []*models.Bar

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
		return percentage * (a.Market.BaseAsset.Quantity * a.Market.Price.Close)
	}
}

// Calculate the current % profit of the position vs
func (algo *Algo) CurrentProfit(price float64) float64 {
	//TODO this doesnt work on a spot backtest
	if algo.Market.QuoteAsset.Quantity == 0 {
		return 0
	} else if algo.Market.QuoteAsset.Quantity < 0 {
		return calculateDifference(algo.Market.AverageCost, price)
	} else {
		return calculateDifference(price, algo.Market.AverageCost)
	}
}

func (algo *Algo) getExitOrderSize(orderSizeGreaterThanPositionSize bool) float64 {
	if orderSizeGreaterThanPositionSize {
		return algo.Market.Leverage
	} else {
		return algo.ExitOrderSize
	}
}

func (algo *Algo) getEntryOrderSize(orderSizeGreaterThanMaxPositionSize bool) float64 {
	if orderSizeGreaterThanMaxPositionSize {
		return algo.LeverageTarget - algo.Market.Leverage //-algo.LeverageTarget
	} else {
		return algo.EntryOrderSize
	}
}

func (algo *Algo) canBuy() float64 {
	if algo.CanBuyBasedOnMax {
		return (algo.Market.BaseAsset.Quantity * algo.Market.Price.Open) * algo.Market.MaxLeverage
	} else {
		return (algo.Market.BaseAsset.Quantity * algo.Market.Price.Open) * algo.LeverageTarget
	}
}

//Log the state of the algo and update variables like leverage
func (algo *Algo) logState(timestamp ...string) (state models.History) {
	// algo.History.Timestamp = append(algo.History.Timestamp, timestamp)
	var balance float64
	if algo.Market.Futures {
		balance = algo.Market.BaseAsset.Quantity
		algo.Market.Leverage = math.Abs(algo.Market.QuoteAsset.Quantity) / (algo.Market.Price.Close * balance)
	} else {
		if algo.Market.AverageCost == 0 {
			algo.Market.AverageCost = algo.Market.Price.Close
		}
		balance = (algo.Market.BaseAsset.Quantity * algo.Market.Price.Close) + algo.Market.QuoteAsset.Quantity
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		algo.Market.Leverage = (algo.Market.BaseAsset.Quantity * algo.Market.Price.Close) / balance
		// log.Println("BaseAsset Quantity", algo.Market.BaseAsset.Quantity, "QuoteAsset Value", algo.Market.QuoteAsset.Quantity/algo.Market.Price)
		// log.Println("BaseAsset Value", algo.Market.BaseAsset.Quantity*algo.Market.Price, "QuoteAsset Quantity", algo.Market.QuoteAsset.Quantity)
		// log.Println("Leverage", algo.Market.Leverage)
	}

	// fmt.Println(algo.Timestamp, "Funds", algo.Market.BaseAsset.Quantity, "Quantity", algo.Market.QuoteAsset.Quantity)
	// fmt.Println(algo.Timestamp, algo.Market.BaseAsset.Quantity, algo.CurrentProfit(algo.Market.Price))
	algo.Market.Profit = algo.Market.BaseAsset.Quantity * (algo.CurrentProfit(algo.Market.Price.Close) * algo.Market.Leverage)
	// fmt.Println(algo.Timestamp, algo.Market.Profit)

	if timestamp != nil {
		algo.Timestamp = timestamp[0]
		state = models.History{
			Timestamp:   timestamp[0],
			Balance:     balance,
			Quantity:    algo.Market.QuoteAsset.Quantity,
			AverageCost: algo.Market.AverageCost,
			Leverage:    algo.Market.Leverage,
			Profit:      algo.Market.Profit,
			Price:       algo.Market.Price.Close,
		}

		if algo.Market.Futures {
			if math.IsNaN(algo.Market.Profit) {
				state.UBalance = balance
			} else {
				state.UBalance = balance + algo.Market.Profit
			}
		} else {
			state.UBalance = (algo.Market.BaseAsset.Quantity * algo.Market.Price.Close) + algo.Market.QuoteAsset.Quantity
		}

	} else {
		algo.LogLiveState()
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Market.BaseAsset.Quantity*algo.Market.Price.Close+(algo.Market.QuoteAsset.Quantity), 0, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.Price.Close, algo.Market.AverageCost))
	}
	return
}

func (algo *Algo) getOrderSize(currentPrice float64) (orderSize float64, side float64) {
	currentWeight := math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	if algo.Market.QuoteAsset.Quantity == 0 {
		currentWeight = float64(algo.Market.Weight)
	}
	adding := currentWeight == float64(algo.Market.Weight)
	// fmt.Println(algo.Timestamp, "Here", algo.canBuy(), (algo.Market.QuoteAsset.Quantity))
	if (currentWeight == 0 || adding) && algo.Market.Leverage+algo.DeleverageOrderSize <= algo.LeverageTarget && algo.Market.Weight != 0 {
		orderSize = algo.getEntryOrderSize(algo.EntryOrderSize > algo.LeverageTarget-algo.Market.Leverage)
		side = float64(algo.Market.Weight)
	} else if !adding {
		orderSize = algo.getExitOrderSize(algo.ExitOrderSize > algo.Market.Leverage && algo.Market.Weight == 0)
		side = float64(currentWeight * -1)
	} else if math.Abs(algo.Market.QuoteAsset.Quantity) > algo.canBuy()*(1+algo.DeleverageOrderSize) && adding {
		orderSize = algo.DeleverageOrderSize
		side = float64(currentWeight * -1)
	} else if algo.Market.Weight == 0 && algo.Market.Leverage > 0 {
		orderSize = algo.getExitOrderSize(algo.ExitOrderSize > algo.Market.Leverage)
		//side = Opposite of the quantity
		side = -math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	} else if algo.canBuy() > math.Abs(algo.Market.QuoteAsset.Quantity) {
		// If I can buy more, place order to fill diff of canBuy and current quantity
		orderSize = calculateDifference(algo.canBuy(), math.Abs(algo.Market.QuoteAsset.Quantity))
		side = float64(algo.Market.Weight)
	}
	return
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
func CreateSpread(weight int32, confidence float64, price float64, spread float64, TickSize float64, maxOrders int32) models.OrderArray {
	xStart := 0.0
	if weight == 1 {
		xStart = price - (price * spread)
	} else {
		xStart = price
	}
	xStart = Round(xStart, TickSize)

	xEnd := xStart + (xStart * spread)
	xEnd = Round(xEnd, TickSize)

	diff := xEnd - xStart

	if diff/TickSize >= float64(maxOrders) {
		newTickSize := diff / (float64(maxOrders) - 1)
		TickSize = Round(newTickSize, TickSize)
	}

	var priceArr []float64

	if weight == 1 {
		priceArr = Arange(xStart, xEnd-float64(TickSize), float64(TickSize))
	} else {
		priceArr = Arange(xStart, xEnd, float64(TickSize))
	}

	temp := divArr(priceArr, xStart)

	dist := expArr(temp, confidence)
	normalizer := 1 / sumArr(dist)
	orderArr := mulArr(dist, normalizer)
	if weight == 1 {
		orderArr = ReverseArr(orderArr)
	}
	return models.OrderArray{Price: priceArr, Quantity: orderArr}
}

// Break down the bars into open, high, low, close arrays that are easier to manipulate.
func GetOHLCBars(bars []*models.Bar) ([]float64, []float64, []float64, []float64) {
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

func ToIntTimestamp(timeString string) int {
	layout := "2006-01-02 15:04:05"
	currentTime, err := time.Parse(layout, timeString)
	if err != nil {
		fmt.Printf("Error parsing timeString: %v\n", err)
	}
	return int(currentTime.UnixNano() / int64(time.Millisecond))
}

func ToTimeObject(timeString string) time.Time {
	layout := "2006-01-02 15:04:05"
	if strings.Contains(timeString, "+0000 UTC") {
		timeString = strings.Replace(timeString, "+0000 UTC", "", 1)
		fmt.Printf("Trimmed timestring: %v\n", timeString)
	}
	timeString = strings.TrimSpace(timeString)
	currentTime, err := time.Parse(layout, timeString)
	if err != nil {
		fmt.Printf("Error parsing timeString: %v", err)
	}
	return currentTime
}

func TimestampToTime(timestamp int) time.Time {
	timeInt, err := strconv.ParseInt(strconv.Itoa(timestamp/1000), 10, 64)
	if err != nil {
		panic(err)
	}
	return time.Unix(timeInt, 0)
}

func TimeToTimestamp(timeObject time.Time) int {
	return timeObject.UTC().Nanosecond() / 1000000
}

func Round(x, unit float64) float64 {
	return math.Round(x/unit) * unit
}

func ReverseArr(a []float64) []float64 {
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func Arange(min float64, max float64, step float64) []float64 {
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

func ToFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func RoundToNearest(num float64, interval float64) float64 {
	return math.Round(num/interval) * interval
}

func AdjustForSlippage(premium float64, side string, slippage float64) float64 {
	adjPremium := premium
	if side == "buy" {
		adjPremium = premium * (1 + (slippage / 100.))
	} else if side == "sell" {
		adjPremium = premium * (1 - (slippage / 100.))
	}
	return adjPremium
}

func GetDeribitOptionSymbol(expiry int, strike float64, currency string, optionType string) string {
	expiryTime := time.Unix(int64(expiry/1000), 0)
	year, month, day := expiryTime.Date()
	return "BTC-" + string(day) + string(month) + string(year) + "-" + optionType
}

func GetNextFriday(currentTime time.Time) time.Time {
	dayDiff := currentTime.Weekday()
	if dayDiff <= 0 {
		dayDiff += 7
	}
	return currentTime.Truncate(24 * time.Hour).Add(time.Hour * time.Duration(24*dayDiff))
}

func GetLastFridayOfMonth(currentTime time.Time) time.Time {
	year, month, _ := currentTime.Date()
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1).Day()
	currentTime = time.Date(year, month, lastOfMonth, 0, 0, 0, 0, time.UTC)
	for i := lastOfMonth; i > 0; i-- {
		if currentTime.Weekday() == 5 {
			return currentTime
		}
		currentTime = currentTime.Add(-time.Hour * time.Duration(24))
	}
	return currentTime
}

func GetQuarterlyExpiry(currentTime time.Time, minDays int) time.Time {
	year, month, _ := currentTime.Add(time.Hour * time.Duration(24*minDays)).Date()
	// Get nearest quarterly month
	quarterlyMonth := month + (month % 4)
	if quarterlyMonth >= 12 {
		year += 1
		quarterlyMonth = quarterlyMonth % 12
	}
	lastFriday := GetLastFridayOfMonth(time.Date(year, month, 1, 0, 0, 0, 0, time.UTC))
	// fmt.Printf("Got quarterly expiry %v\n", lastFriday)
	return lastFriday
}
