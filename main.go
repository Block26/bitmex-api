// Package yantra is designed to be an algorithmic creation engine.
// There are three layers in which and algorithm is created using yanta.
//
// 1) Yantra, the testing and live trading engine.
//
// 2) The logical operations. ex) buy if sma > ema
//
// 3) The parameters and ordering of the logic.
package yantra

import (
	"fmt"
	"log"
	"math"
	"time"

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"
	"github.com/tantralabs/yantra/exchanges"
)

// Algo is where you will define your initial state and it will keep track of your state throughout a test and during live execution.
type Algo struct {
	Name              string                 // A UUID that tells the live system what algorithm is executing trades
	Market            models.Market          // The market this Algo is trading. Refer to exchanges.LoadMarket(exchange, symbol)
	FillType          string                 // The simulation fill type for this Algo. Refer to exchanges.FillType() for options
	RebalanceInterval string                 // The interval at which rebalance should be called. Refer to exchanges.RebalanceInterval() for options
	Debug             bool                   // Turn logs on or off
	Index             int                    // Current index of the Algo in it's data
	Timestamp         string                 // Current timestamp of the Algo in it's data
	DataLength        int                    // Datalength tells the Algo when it is safe to start rebalancing, your Datalength should be longer than any TA length
	History           []models.History       // Used to Store historical states
	Params            map[string]interface{} // Save the initial Params of the Algo, for logging purposes. This is used to check the params after running a genetic search.
	Result            map[string]interface{} // The result of your backtest
	LogBacktestToCSV  bool                   // Exports the backtest history to a balance.csv in your local directory
	State             map[string]interface{} // State of the algo, useful for logging live ta indicators.

	// AutoOrderPlacement
	// AutoOrderPlacement is not neccesary and can be false, it is the easiest way to create an algorithm with yantra
	// using AutoOrderPlacement will allow yantra to automatically leverage and deleverage your account based on Algo.Market.Weight
	//
	// Examples
	// EX) If RebalanceInterval().Hour and EntryOrderSize = 0.1 then when you are entering a position you will order 10% of your LeverageTarget per hour.
	// EX) If RebalanceInterval().Hour and ExitOrderSize = 0.1 then when you are exiting a position you will order 10% of your LeverageTarget per hour.
	// EX) If RebalanceInterval().Hour and DeleverageOrderSize = 0.01 then when you are over leveraged you will order 1% of your LeverageTarget per hour until you are no longer over leveraged.
	// EX) If Market.MaxLeverage is 1 and Algo.LeverageTarget is 1 then your algorithm will be fully leveraged when it enters it's position.
	AutoOrderPlacement  bool    // AutoOrderPlacement whether yantra should manage your orders / leverage for you.
	CanBuyBasedOnMax    bool    // If true then yantra will calculate leverage based on Market.MaxLeverage, if false then yantra will calculate leverage based on Algo.LeverageTarget
	LeverageTarget      float64 // The target leverage for the Algo, 1 would be 100%, 0.5 would be 50% of the MaxLeverage defined by Market.
	EntryOrderSize      float64 // The speed at which the algo enters positions during the RebalanceInterval
	ExitOrderSize       float64 // The speed at which the algo exits positions during the RebalanceInterval
	DeleverageOrderSize float64 // The speed at which the algo exits positions during the RebalanceInterval if it is over leveraged, current leverage is determined by Algo.LeverageTarget or Market.MaxLeverage.
}

// SetLiquidity Set the liquidity available for to buy/sell. IE put 5% of my portfolio on the bid.
func (algo *Algo) SetLiquidity(percentage float64, side int) float64 {
	if algo.Market.Futures {
		return percentage * algo.Market.BaseAsset.Quantity
	} else {
		if side == 1 {
			return percentage * algo.Market.QuoteAsset.Quantity
		}
		return percentage * (algo.Market.BaseAsset.Quantity * algo.Market.Price.Close)
	}
}

// CurrentProfit Calculate the current % profit of the position vs
func (algo *Algo) CurrentProfit(price float64) float64 {
	//TODO this doesnt work on a spot backtest
	if algo.Market.QuoteAsset.Quantity == 0 {
		return 0
	} else if algo.Market.QuoteAsset.Quantity < 0 {
		return utils.CalculateDifference(algo.Market.AverageCost, price)
	} else {
		return utils.CalculateDifference(price, algo.Market.AverageCost)
	}
}

func (algo *Algo) getPositionAbsLoss() float64 {
	positionLoss := 0.0
	if algo.Market.QuoteAsset.Quantity < 0 {
		positionLoss = algo.Market.BaseAsset.Quantity * (algo.CurrentProfit(algo.Market.Price.High) * algo.Market.Leverage)
	} else {
		positionLoss = algo.Market.BaseAsset.Quantity * (algo.CurrentProfit(algo.Market.Price.Low) * algo.Market.Leverage)
	}
	return positionLoss
}

func (algo *Algo) getPositionAbsProfit() float64 {
	positionProfit := 0.0
	if algo.Market.QuoteAsset.Quantity > 0 {
		positionProfit = algo.Market.BaseAsset.Quantity * (algo.CurrentProfit(algo.Market.Price.High) * algo.Market.Leverage)
	} else {
		positionProfit = algo.Market.BaseAsset.Quantity * (algo.CurrentProfit(algo.Market.Price.Low) * algo.Market.Leverage)
	}
	return positionProfit
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
func (algo *Algo) logState(timestamp ...time.Time) (state models.History) {
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
		algo.Timestamp = timestamp[0].String()
		state = models.History{
			Timestamp:   algo.Timestamp,
			Balance:     balance,
			Quantity:    algo.Market.QuoteAsset.Quantity,
			AverageCost: algo.Market.AverageCost,
			Leverage:    algo.Market.Leverage,
			Profit:      algo.Market.Profit,
			MaxLoss:     algo.getPositionAbsLoss(),
			MaxProfit:   algo.getPositionAbsProfit(),
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
		algo.logLiveState()
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Market.BaseAsset.Quantity*algo.Market.Price.Close+(algo.Market.QuoteAsset.Quantity), 0.0, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.Price.Close, algo.Market.AverageCost))
	}
	return
}

func (algo *Algo) getOrderSize(currentPrice float64, live ...bool) (orderSize float64, side float64) {
	currentWeight := math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	if algo.Market.QuoteAsset.Quantity == 0 {
		currentWeight = float64(algo.Market.Weight)
	}
	adding := currentWeight == float64(algo.Market.Weight)
	// fmt.Printf("CURRENT WEIGHT %v, adding %v, leverage target %v, can buy %v, deleverage order size %v\n", currentWeight, adding, algo.LeverageTarget, algo.canBuy(), algo.DeleverageOrderSize)
	// fmt.Printf("Getting order size with quote asset quantity: %v\n", algo.Market.QuoteAsset.Quantity)

	// Change order sizes for live to ensure similar boolen checks
	exitOrderSize := algo.ExitOrderSize
	entryOrderSize := algo.EntryOrderSize
	
	if live != nil && live[0] {
		if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
			exitOrderSize = algo.ExitOrderSize / 60
			entryOrderSize = algo.EntryOrderSize / 60
		}
	}

	if (currentWeight == 0 || adding) && algo.Market.Leverage+algo.DeleverageOrderSize <= algo.LeverageTarget && algo.Market.Weight != 0 {
		// fmt.Printf("Getting entry order with entry order size %v, leverage target %v, leverage %v\n", entryOrderSize, algo.LeverageTarget, algo.Market.Leverage)
		orderSize = algo.getEntryOrderSize(entryOrderSize > algo.LeverageTarget-algo.Market.Leverage)
		side = float64(algo.Market.Weight)
	} else if !adding {
		// fmt.Printf("Getting exit order size with exit order size %v, leverage %v, weight %v\n", exitOrderSize, algo.Market.Leverage, algo.Market.Weight)
		orderSize = algo.getExitOrderSize(exitOrderSize > algo.Market.Leverage && algo.Market.Weight == 0)
		side = float64(currentWeight * -1)
	} else if math.Abs(algo.Market.QuoteAsset.Quantity) > algo.canBuy()*(1+algo.DeleverageOrderSize) && adding {
		orderSize = algo.DeleverageOrderSize
		side = float64(currentWeight * -1)
	} else if algo.Market.Weight == 0 && algo.Market.Leverage > 0 {
		orderSize = algo.getExitOrderSize(exitOrderSize > algo.Market.Leverage)
		//side = Opposite of the quantity
		side = -math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	} else if algo.canBuy() > math.Abs(algo.Market.QuoteAsset.Quantity) {
		// If I can buy more, place order to fill diff of canBuy and current quantity
		orderSize = utils.CalculateDifference(algo.canBuy(), math.Abs(algo.Market.QuoteAsset.Quantity))
		side = float64(algo.Market.Weight)
	}
	return
}

//Log the state of the algo to influx db
func (algo *Algo) logLiveState() {
	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://ec2-54-219-145-3.us-west-1.compute.amazonaws.com:8086",
		Username: "russell",
		Password: "KNW(12nAS921D",
	})

	if err != nil {
		log.Fatal(err)
	}

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": algo.Name, "commit_hash": commitHash}

	fields := structs.Map(algo.Market)

	//TODO: shouldn't have to manually delete Options param here
	_, ok := fields["Options"]
	if ok {
		delete(fields, "Options")
	}

	fields["Price"] = algo.Market.Price.Close
	fields["Balance"] = algo.Market.BaseAsset.Quantity
	fields["Quantity"] = algo.Market.QuoteAsset.Quantity

	pt, err := client.NewPoint(
		"market",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	fields = algo.Params

	fields["EntryOrderSize"] = algo.EntryOrderSize
	fields["ExitOrderSize"] = algo.ExitOrderSize
	fields["DeleverageOrderSize"] = algo.DeleverageOrderSize

	pt, err = client.NewPoint(
		"params",
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
		if err != nil {
			log.Fatal(err)
		}
		bp.AddPoint(pt)
	}

	err = client.Client.Write(influx, bp)
	if err != nil {
		log.Fatal(err)
	}
	influx.Close()
}

// CreateSpread Create a Spread on the bid/ask, this fuction is used to create an arrary of orders that spreads across the order book.
func (algo *Algo) CreateSpread(weight int32, confidence float64, price float64, spread float64) models.OrderArray {
	tickSize := algo.Market.TickSize
	maxOrders := algo.Market.MaxOrders
	xStart := 0.0
	if weight == 1 {
		xStart = price - (price * spread)
	} else {
		xStart = price
	}
	xStart = utils.Round(xStart, tickSize)

	xEnd := xStart + (xStart * spread)
	xEnd = utils.Round(xEnd, tickSize)

	diff := xEnd - xStart

	if diff/tickSize >= float64(maxOrders) {
		newTickSize := diff / (float64(maxOrders) - 1)
		tickSize = utils.Round(newTickSize, tickSize)
	}

	var priceArr []float64

	if weight == 1 {
		priceArr = utils.Arange(xStart, xEnd-float64(tickSize), float64(tickSize))
	} else {
		if xStart-xEnd < float64(tickSize) {
			xEnd = xEnd + float64(tickSize)
		}
		priceArr = utils.Arange(xStart, xEnd, float64(tickSize))
	}

	temp := utils.DivArr(priceArr, xStart)

	dist := utils.ExpArr(temp, confidence)
	normalizer := 1 / utils.SumArr(dist)
	orderArr := utils.MulArr(dist, normalizer)
	if weight == 1 {
		orderArr = utils.ReverseArr(orderArr)
	}
	return models.OrderArray{Price: priceArr, Quantity: orderArr}
}
