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
	"strings"
	"time"

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
	. "github.com/tantralabs/models"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/utils"
)

// SetLiquidity Set the liquidity available for to buy/sell. IE put 5% of my portfolio on the bid.
func SetLiquidity(algo *Algo, percentage float64, side int) float64 {
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
func CurrentProfit(algo *Algo, price float64) float64 {
	//TODO this doesnt work on a spot backtest
	if algo.Market.QuoteAsset.Quantity == 0 {
		return 0
	} else if algo.Market.QuoteAsset.Quantity < 0 {
		return utils.CalculateDifference(algo.Market.AverageCost, price)
	} else {
		return utils.CalculateDifference(price, algo.Market.AverageCost)
	}
}

func getPositionAbsLoss(algo *Algo) float64 {
	positionLoss := 0.0
	if algo.Market.QuoteAsset.Quantity < 0 {
		positionLoss = (algo.Market.BaseAsset.Quantity * (CurrentProfit(algo, algo.Market.Price.High) * algo.Market.Leverage)) + CurrentOptionProfit(algo)
	} else {
		positionLoss = (algo.Market.BaseAsset.Quantity * (CurrentProfit(algo, algo.Market.Price.Low) * algo.Market.Leverage)) + CurrentOptionProfit(algo)
	}
	return positionLoss
}

func getPositionAbsProfit(algo *Algo) float64 {
	positionProfit := 0.0
	if algo.Market.QuoteAsset.Quantity > 0 {
		positionProfit = (algo.Market.BaseAsset.Quantity * (CurrentProfit(algo, algo.Market.Price.High) * algo.Market.Leverage)) + CurrentOptionProfit(algo)
	} else {
		positionProfit = (algo.Market.BaseAsset.Quantity * (CurrentProfit(algo, algo.Market.Price.Low) * algo.Market.Leverage)) + CurrentOptionProfit(algo)
	}
	return positionProfit
}

func getExitOrderSize(algo *Algo, orderSizeGreaterThanPositionSize bool) float64 {
	if orderSizeGreaterThanPositionSize {
		return algo.Market.Leverage
	} else {
		return algo.ExitOrderSize
	}
}

func getEntryOrderSize(algo *Algo, orderSizeGreaterThanMaxPositionSize bool) float64 {
	if orderSizeGreaterThanMaxPositionSize {
		return algo.LeverageTarget - algo.Market.Leverage //-algo.LeverageTarget
	} else {
		return algo.EntryOrderSize
	}
}

func canBuy(algo *Algo) float64 {
	if algo.CanBuyBasedOnMax {
		return (algo.Market.BaseAsset.Quantity * algo.Market.Price.Open) * algo.Market.MaxLeverage
	} else {
		return (algo.Market.BaseAsset.Quantity * algo.Market.Price.Open) * algo.LeverageTarget
	}
}

//Log the state of the algo and update variables like leverage
func logState(algo *Algo, timestamp ...time.Time) (state History) {
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
		// log.Println("BaseAsset Value", algo.Market.BaseAsset.Quantity*Algo.Market.Price, "QuoteAsset Quantity", algo.Market.QuoteAsset.Quantity)
		// log.Println("Leverage", algo.Market.Leverage)
	}

	// fmt.Println(algo.Timestamp, "Funds", algo.Market.BaseAsset.Quantity, "Quantity", algo.Market.QuoteAsset.Quantity)
	// fmt.Println(algo.Timestamp, algo.Market.BaseAsset.Quantity, algo.CurrentProfit(algo.Market.Price))
	algo.Market.Profit = (algo.Market.BaseAsset.Quantity * (CurrentProfit(algo, algo.Market.Price.Close) * algo.Market.Leverage)) + CurrentOptionProfit(algo)
	// fmt.Println(algo.Timestamp, algo.Market.Profit)

	if timestamp != nil {
		algo.Timestamp = timestamp[0].String()
		state = History{
			Timestamp:   algo.Timestamp,
			Balance:     balance,
			Quantity:    algo.Market.QuoteAsset.Quantity,
			AverageCost: algo.Market.AverageCost,
			Leverage:    algo.Market.Leverage,
			Profit:      algo.Market.Profit,
			Weight:      algo.Market.Weight,
			MaxLoss:     getPositionAbsLoss(algo),
			MaxProfit:   getPositionAbsProfit(algo),
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
		logLiveState(algo)
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Market.BaseAsset.Quantity*algo.Market.Price.Close+(algo.Market.QuoteAsset.Quantity), 0.0, algo.Market.BaseAsset.Quantity, algo.Market.QuoteAsset.Quantity, algo.Market.Price.Close, algo.Market.AverageCost))
	}
	return
}

func getOrderSize(algo *Algo, currentPrice float64, live ...bool) (orderSize float64, side float64) {
	currentWeight := math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	if algo.Market.QuoteAsset.Quantity == 0 {
		currentWeight = float64(algo.Market.Weight)
	}
	adding := currentWeight == float64(algo.Market.Weight)
	// fmt.Printf("CURRENT WEIGHT %v, adding %v, leverage target %v, can buy %v, deleverage order size %v\n", currentWeight, adding, algo.LeverageTarget, canBuy(algo), algo.DeleverageOrderSize)
	// fmt.Printf("Getting order size with quote asset quantity: %v\n", algo.Market.QuoteAsset.Quantity)

	// Change order sizes for live to ensure similar boolen checks
	exitOrderSize := algo.ExitOrderSize
	entryOrderSize := algo.EntryOrderSize
	deleverageOrderSize := algo.DeleverageOrderSize

	if live != nil && live[0] {
		if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
			exitOrderSize = algo.ExitOrderSize / 60
			entryOrderSize = algo.EntryOrderSize / 60
			deleverageOrderSize = algo.DeleverageOrderSize / 60
		}
	}

	if (currentWeight == 0 || adding) && algo.Market.Leverage+algo.DeleverageOrderSize <= algo.LeverageTarget && algo.Market.Weight != 0 {
		// fmt.Printf("Getting entry order with entry order size %v, leverage target %v, leverage %v\n", entryOrderSize, algo.LeverageTarget, algo.Market.Leverage)
		orderSize = getEntryOrderSize(algo, entryOrderSize > algo.LeverageTarget-algo.Market.Leverage)
		side = float64(algo.Market.Weight)
	} else if !adding {
		// fmt.Printf("Getting exit order size with exit order size %v, leverage %v, weight %v\n", exitOrderSize, algo.Market.Leverage, algo.Market.Weight)
		orderSize = getExitOrderSize(algo, exitOrderSize > algo.Market.Leverage && algo.Market.Weight == 0)
		side = float64(currentWeight * -1)
	} else if math.Abs(algo.Market.QuoteAsset.Quantity) > canBuy(algo)*(1+deleverageOrderSize) && adding {
		orderSize = algo.DeleverageOrderSize
		side = float64(currentWeight * -1)
	} else if algo.Market.Weight == 0 && algo.Market.Leverage > 0 {
		orderSize = getExitOrderSize(algo, exitOrderSize > algo.Market.Leverage)
		//side = Opposite of the quantity
		side = -math.Copysign(1, algo.Market.QuoteAsset.Quantity)
	} else if canBuy(algo) > math.Abs(algo.Market.QuoteAsset.Quantity) {
		// If I can buy more, place order to fill diff of canBuy and current quantity
		orderSize = utils.CalculateDifference(canBuy(algo), math.Abs(algo.Market.QuoteAsset.Quantity))
		side = float64(algo.Market.Weight)
	}
	return
}

func getFillPrice(algo *Algo, bar *Bar) float64 {
	var fillPrice float64
	if algo.FillType == exchanges.FillType().Worst {
		if algo.Market.Weight > 0 && algo.Market.QuoteAsset.Quantity > 0 {
			fillPrice = bar.High
		} else if algo.Market.Weight < 0 && algo.Market.QuoteAsset.Quantity < 0 {
			fillPrice = bar.Low
		} else if algo.Market.Weight != 1 && algo.Market.QuoteAsset.Quantity > 0 {
			fillPrice = bar.Low
		} else if algo.Market.Weight != -1 && algo.Market.QuoteAsset.Quantity < 0 {
			fillPrice = bar.High
		} else {
			fillPrice = bar.Close
		}
	} else if algo.FillType == exchanges.FillType().Close {
		fillPrice = bar.Close
	} else if algo.FillType == exchanges.FillType().Open {
		fillPrice = bar.Open
	} else if algo.FillType == exchanges.FillType().MeanOC {
		fillPrice = (bar.Open + bar.Close) / 2
	} else if algo.FillType == exchanges.FillType().MeanHL {
		fillPrice = (bar.High + bar.Low) / 2
	}
	return fillPrice
}

func getInfluxClient() client.Client {
	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     "http://ec2-54-219-145-3.us-west-1.compute.amazonaws.com:8086",
		Username: "russell",
		Password: "KNW(12nAS921D",
	})

	if err != nil {
		fmt.Println("err", err)
	}

	return influx
}

func logTrade(algo *Algo, trade iex.Order) {
	stateType := "live"
	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": algo.Name, "commit_hash": commitHash, "state_type": stateType, "side": strings.ToLower(trade.Side)}

	fields := structs.Map(trade)
	pt, err := client.NewPoint(
		"trades",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()
}

func logFilledTrade(algo *Algo, trade iex.Order) {
	stateType := "live"
	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": algo.Name, "commit_hash": commitHash, "state_type": stateType, "side": strings.ToLower(trade.Side)}

	fields := structs.Map(trade)
	pt, err := client.NewPoint(
		"filled_trades",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	err = client.Client.Write(influx, bp)
	if err != nil {
		fmt.Println("err", err)
	}
	influx.Close()
}

//Log the state of the algo to influx db
func logLiveState(algo *Algo, test ...bool) {
	stateType := "live"
	if test != nil {
		stateType = "test"
	}

	influx := getInfluxClient()

	bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
		Database:  "algos",
		Precision: "us",
	})

	tags := map[string]string{"algo_name": algo.Name, "commit_hash": commitHash, "state_type": stateType}

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

	if algo.AutoOrderPlacement {
		fields["EntryOrderSize"] = algo.EntryOrderSize
		fields["ExitOrderSize"] = algo.ExitOrderSize
		fields["DeleverageOrderSize"] = algo.DeleverageOrderSize
		fields["LeverageTarget"] = algo.LeverageTarget
		fields["ShouldHaveQuantity"] = algo.ShouldHaveQuantity
		fields["FillPrice"] = algo.FillPrice
	}

	pt, err = client.NewPoint(
		"params",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	// LOG Options
	for _, option := range algo.Market.OptionContracts {
		if option.Position != 0 {
			tmpTags := tags
			tmpTags["symbol"] = option.Symbol
			o := structs.Map(option.OptionTheo)
			pt1, _ := client.NewPoint(
				"optionTheo",
				tmpTags,
				o,
				time.Now(),
			)
			bp.AddPoint(pt1)

			o = structs.Map(option)
			delete(o, "OptionTheo")
			pt2, _ := client.NewPoint(
				"options",
				tmpTags,
				o,
				time.Now(),
			)
			bp.AddPoint(pt2)
		}
	}

	// LOG orders placed
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
		fmt.Println("err", err)
	}
	influx.Close()
}

// CreateSpread Create a Spread on the bid/ask, this fuction is used to create an arrary of orders that spreads across the order book.
func CreateSpread(algo *Algo, weight int32, confidence float64, price float64, spread float64) OrderArray {
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
	return OrderArray{Price: priceArr, Quantity: orderArr}
}
