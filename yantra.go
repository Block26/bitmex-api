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
	"os"
	"strings"
	"time"

	"github.com/fatih/structs"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/exchanges"
	"github.com/tantralabs/yantra/models"
	. "github.com/tantralabs/yantra/models"
	"github.com/tantralabs/tradeapi/iex"
	"github.com/tantralabs/utils"
)

// SetLiquidity Set the liquidity available for to buy/sell. IE put 5% of my portfolio on the bid.
func SetLiquidity(algo *Algo, marketState *MarketState, percentage float64, side int) float64 {
	if marketState.Info.MarketType == Future {
		return percentage * algo.Account.BaseAsset.Quantity
	} else {
		if side == 1 {
			return percentage * marketState.Position
		}
		return percentage * marketState.Position * marketState.Bar.Close
	}
}

// CurrentProfit Calculate the current % profit of the position vs
func CurrentProfit(marketState *MarketState, price float64) float64 {
	//TODO this doesnt work on a spot backtest
	if marketState.Position == 0 {
		return 0
	} else if marketState.Position < 0 {
		return utils.CalculateDifference(marketState.AverageCost, price)
	} else {
		return utils.CalculateDifference(price, marketState.AverageCost)
	}
}

func getPositionAbsLoss(algo *Algo, marketState *MarketState) float64 {
	positionLoss := 0.0
	if marketState.Position < 0 {
		positionLoss = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
	} else {
		positionLoss = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
	}
	return positionLoss
}

func getPositionAbsProfit(algo *Algo, marketState *MarketState) float64 {
	positionProfit := 0.0
	if marketState.Position > 0 {
		positionProfit = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.High) * marketState.Leverage))
	} else {
		positionProfit = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Low) * marketState.Leverage))
	}
	return positionProfit
}

func getExitOrderSize(marketState *MarketState, orderSizeGreaterThanPositionSize bool) float64 {
	if orderSizeGreaterThanPositionSize {
		return marketState.Leverage
	} else {
		return marketState.ExitOrderSize
	}
}

func getEntryOrderSize(marketState *MarketState, orderSizeGreaterThanMaxPositionSize bool) float64 {
	if orderSizeGreaterThanMaxPositionSize {
		return marketState.LeverageTarget - marketState.Leverage //-marketState.LeverageTarget
	} else {
		return marketState.EntryOrderSize
	}
}

func canBuy(algo *Algo, marketState *MarketState) float64 {
	if marketState.CanBuyBasedOnMax {
		return (algo.Account.BaseAsset.Quantity * marketState.Bar.Open) * marketState.MaxLeverage
	} else {
		return (algo.Account.BaseAsset.Quantity * marketState.Bar.Open) * marketState.LeverageTarget
	}
}

//Log the state of the algo and update variables like leverage
func logState(algo *Algo, marketState *MarketState, timestamp ...time.Time) (state History) {
	// algo.History.Timestamp = append(algo.History.Timestamp, timestamp)
	var balance float64
	if marketState.Info.MarketType == Future {
		balance = algo.Account.BaseAsset.Quantity
		marketState.Leverage = math.Abs(marketState.Position) / (marketState.Bar.Close * balance)
	} else {
		if marketState.AverageCost == 0 {
			marketState.AverageCost = marketState.Bar.Close
		}
		balance = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		// TODO need to define an ideal delta if not trading futures ie do you want 0%, 50% or 100% of the quote curreny
		marketState.Leverage = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) / balance
		// log.Println("BaseAsset Quantity", algo.Account.BaseAsset.Quantity, "QuoteAsset Value", marketState.Position/marketState.Bar)
		// log.Println("BaseAsset Value", algo.Account.BaseAsset.Quantity*marketState.Bar, "QuoteAsset Quantity", marketState.Position)
		// log.Println("Leverage", marketState.Leverage)
	}

	// fmt.Println(algo.Timestamp, "Funds", algo.Account.BaseAsset.Quantity, "Quantity", marketState.Position)
	// fmt.Println(algo.Timestamp, algo.Account.BaseAsset.Quantity, algo.CurrentProfit(marketState.Bar))
	marketState.Profit = (algo.Account.BaseAsset.Quantity * (CurrentProfit(marketState, marketState.Bar.Close) * marketState.Leverage))
	// fmt.Println(algo.Timestamp, marketState.Profit)

	if timestamp != nil {
		algo.Timestamp = timestamp[0]
		state = models.History{
			Timestamp:   algo.Timestamp.String(),
			Balance:     balance,
			Quantity:    marketState.Position,
			AverageCost: marketState.AverageCost,
			Leverage:    marketState.Leverage,
			Profit:      marketState.Profit,
			Weight:      int(marketState.Weight),
			MaxLoss:     getPositionAbsLoss(algo, marketState),
			MaxProfit:   getPositionAbsProfit(algo, marketState),
			Price:       marketState.Bar.Close,
		}

		if marketState.Info.MarketType == Future {
			if math.IsNaN(marketState.Profit) {
				state.UBalance = balance
			} else {
				state.UBalance = balance + marketState.Profit
			}
		} else {
			state.UBalance = (algo.Account.BaseAsset.Quantity * marketState.Bar.Close) + marketState.Position
		}

	} else {
	}
	if algo.Debug {
		fmt.Print(fmt.Sprintf("Portfolio Value %0.2f | Delta %0.2f | Base %0.2f | Quote %.2f | Price %.5f - Cost %.5f \n", algo.Account.BaseAsset.Quantity*marketState.Bar.Close+(marketState.Position), 0.0, algo.Account.BaseAsset.Quantity, marketState.Position, marketState.Bar.Close, marketState.AverageCost))
	}
	return
}

func getOrderSize(algo *Algo, marketState *MarketState, currentPrice float64, live ...bool) (orderSize float64, side float64) {
	currentWeight := math.Copysign(1, marketState.Position)
	if marketState.Position == 0 {
		currentWeight = float64(marketState.Weight)
	}
	adding := currentWeight == float64(marketState.Weight)
	// fmt.Printf("CURRENT WEIGHT %v, adding %v, leverage target %v, can buy %v, deleverage order size %v\n", currentWeight, adding, marketState.LeverageTarget, canBuy(algo), marketState.DeleverageOrderSize)
	// fmt.Printf("Getting order size with quote asset quantity: %v\n", marketState.Position)

	// Change order sizes for live to ensure similar boolen checks
	exitOrderSize := marketState.ExitOrderSize
	entryOrderSize := marketState.EntryOrderSize
	deleverageOrderSize := marketState.DeleverageOrderSize

	if live != nil && live[0] {
		if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
			exitOrderSize = marketState.ExitOrderSize / 60
			entryOrderSize = marketState.EntryOrderSize / 60
			deleverageOrderSize = marketState.DeleverageOrderSize / 60
		}
	}

	if (currentWeight == 0 || adding) && marketState.Leverage+marketState.DeleverageOrderSize <= marketState.LeverageTarget && marketState.Weight != 0 {
		// fmt.Printf("Getting entry order with entry order size %v, leverage target %v, leverage %v\n", entryOrderSize, marketState.LeverageTarget, marketState.Leverage)
		orderSize = getEntryOrderSize(marketState, entryOrderSize > marketState.LeverageTarget-marketState.Leverage)
		side = float64(marketState.Weight)
	} else if !adding {
		// fmt.Printf("Getting exit order size with exit order size %v, leverage %v, weight %v\n", exitOrderSize, marketState.Leverage, marketState.Weight)
		orderSize = getExitOrderSize(marketState, exitOrderSize > marketState.Leverage && marketState.Weight == 0)
		side = float64(currentWeight * -1)
	} else if math.Abs(marketState.Position) > canBuy(algo, marketState)*(1+deleverageOrderSize) && adding {
		orderSize = marketState.DeleverageOrderSize
		side = float64(currentWeight * -1)
	} else if marketState.Weight == 0 && marketState.Leverage > 0 {
		orderSize = getExitOrderSize(marketState, exitOrderSize > marketState.Leverage)
		//side = Opposite of the quantity
		side = -math.Copysign(1, marketState.Position)
	} else if canBuy(algo, marketState) > math.Abs(marketState.Position) {
		// If I can buy more, place order to fill diff of canBuy and current quantity
		orderSize = utils.CalculateDifference(canBuy(algo, marketState), math.Abs(marketState.Position))
		side = float64(marketState.Weight)
	}
	return
}

func getFillPrice(algo *Algo, marketState *MarketState) float64 {
	var fillPrice float64
	if algo.FillType == exchanges.FillType().Worst {
		if marketState.Weight > 0 && marketState.Position > 0 {
			fillPrice = marketState.Bar.High
		} else if marketState.Weight < 0 && marketState.Position < 0 {
			fillPrice = marketState.Bar.Low
		} else if marketState.Weight != 1 && marketState.Position > 0 {
			fillPrice = marketState.Bar.Low
		} else if marketState.Weight != -1 && marketState.Position < 0 {
			fillPrice = marketState.Bar.High
		} else {
			fillPrice = marketState.Bar.Close
		}
	} else if algo.FillType == exchanges.FillType().Close {
		fillPrice = marketState.Bar.Close
	} else if algo.FillType == exchanges.FillType().Open {
		fillPrice = marketState.Bar.Open
	} else if algo.FillType == exchanges.FillType().MeanOC {
		fillPrice = (marketState.Bar.Open + marketState.Bar.Close) / 2
	} else if algo.FillType == exchanges.FillType().MeanHL {
		fillPrice = (marketState.Bar.High + marketState.Bar.Low) / 2
	}
	return fillPrice
}

func getInfluxClient() client.Client {
	influxURL := os.Getenv("YANTRA_LIVE_DB_URL")
	if influxURL == "" {
		log.Fatalln("You need to set the `YANTRA_LIVE_DB_URL` env variable")
	}

	influxUser := os.Getenv("YANTRA_LIVE_DB_USER")
	influxPassword := os.Getenv("YANTRA_LIVE_DB_PASSWORD")

	influx, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     influxURL,
		Username: influxUser,
		Password: influxPassword,
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
func logLiveState(algo *Algo, marketState *MarketState, test ...bool) {
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

	fields := structs.Map(marketState)

	//TODO: shouldn't have to manually delete Options param here
	_, ok := fields["Options"]
	if ok {
		delete(fields, "Options")
	}

	fields["Price"] = marketState.Bar.Close
	fields["Balance"] = algo.Account.BaseAsset.Quantity
	fields["Quantity"] = marketState.Position

	pt, err := client.NewPoint(
		"market",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	fields = algo.Params

	if marketState.AutoOrderPlacement {
		fields["EntryOrderSize"] = marketState.EntryOrderSize
		fields["ExitOrderSize"] = marketState.ExitOrderSize
		fields["DeleverageOrderSize"] = marketState.DeleverageOrderSize
		fields["LeverageTarget"] = marketState.LeverageTarget
		fields["ShouldHaveQuantity"] = marketState.ShouldHaveQuantity
		fields["FillPrice"] = marketState.FillPrice
	}

	pt, err = client.NewPoint(
		"params",
		tags,
		fields,
		time.Now(),
	)
	bp.AddPoint(pt)

	// LOG Options
	for symbol, option := range algo.Account.MarketStates {
		if option.Info.MarketType == Option && option.Position != 0 {
			tmpTags := tags
			tmpTags["symbol"] = symbol
			o := structs.Map(option.OptionTheo)
			// Influxdb seems to interpret pointers as strings, need to dereference here
			o["CurrentTime"] = utils.TimeToTimestamp(*option.OptionTheo.CurrentTime)
			o["UnderlyingPrice"] = *option.OptionTheo.UnderlyingPrice
			pt1, _ := client.NewPoint(
				"optionTheo",
				tmpTags,
				o,
				time.Now(),
			)
			bp.AddPoint(pt1)

			o = structs.Map(option)
			// Influxdb seems to interpret pointers as strings, need to dereference here
			o["CurrentTime"] = (*option.OptionTheo.CurrentTime).String()
			o["UnderlyingPrice"] = *option.OptionTheo.UnderlyingPrice
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

	for _, order := range marketState.Orders {
		fields = map[string]interface{}{
			fmt.Sprintf("%0.2f", order.Rate): order.Amount,
		}

		pt, err = client.NewPoint(
			"order",
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
func CreateSpread(algo *Algo, marketState *MarketState, weight int32, confidence float64, price float64, spread float64) OrderArray {
	tickSize := marketState.Info.PricePrecision
	maxOrders := marketState.Info.MaxOrders
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
