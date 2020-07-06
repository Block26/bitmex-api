package yantra

import (
	"fmt"
	"log"
	"math"
	"os"
	"time"

	"github.com/fatih/structs"
	"github.com/gocarina/gocsv"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/tantralabs/yantra/exchanges"
	"github.com/tantralabs/yantra/models"
	"github.com/tantralabs/yantra/utils"
	"gonum.org/v1/gonum/stat"
)

func getTurnoverStats(history []models.History, algo *models.Algo) models.Stats {
	var previousQuantity float64
	var previousBalance float64

	var profitableDays []float64
	var longPositions []float64
	var shortPositions []float64
	var currentLongPosition []float64
	var currentShortPosition []float64
	var currentLongProfit []float64
	var currentShortProfit []float64

	var longPositionsArr [][]float64
	var shortPositionsArr [][]float64
	var longPositionsProfitArr [][]float64
	var shortPositionsProfitArr [][]float64

	var totalAbsShortProfit float64 = 0.0
	var totalAbsLongProfit float64 = 0.0

	//Get position/turnover stats
	for _, row := range history {
		// Percent profitability of test //
		if history[0].Balance < row.Balance {
			profitableDays = append(profitableDays, 1.0)
		} else {
			profitableDays = append(profitableDays, 0.0)
		}
		//How many long positions did we hold //
		if previousQuantity > 0 && row.Quantity <= 0 {
			longPositions = append(longPositions, previousQuantity)
		}
		//How many short positions did we hold //
		if previousQuantity < 0 && row.Quantity >= 0 {
			shortPositions = append(shortPositions, previousQuantity)
		}
		//Create array of long positions //
		if row.Quantity > 0 {
			currentLongPosition = append(currentLongPosition, row.Quantity)
		} else {
			if len(currentLongPosition) != 0 {
				longPositionsArr = append(longPositionsArr, currentLongPosition)
				currentLongPosition = nil
			}
		}
		//Create array of short positions //
		if row.Quantity < 0 {
			currentShortPosition = append(currentShortPosition, row.Quantity)
		} else {
			if len(currentShortPosition) != 0 {
				shortPositionsArr = append(shortPositionsArr, currentShortPosition)
				currentShortPosition = nil
			}
		}
		// Create arrays of realized long position profit //
		// This is calculated as percent profit
		if row.Quantity >= 0 && row.Quantity < previousQuantity {
			totalAbsLongProfit += row.Balance - previousBalance
			currentLongProfit = append(currentLongProfit, (row.Balance-previousBalance)/previousBalance)
		} else {
			if len(currentLongProfit) != 0 {
				longPositionsProfitArr = append(longPositionsProfitArr, currentLongProfit)
				currentLongProfit = nil
			}
		}
		//Create arrays of realized short position profit //
		// This is calculated as percent profit
		if row.Quantity <= 0 && row.Quantity > previousQuantity {
			totalAbsShortProfit += row.Balance - previousBalance
			currentShortProfit = append(currentShortProfit, (row.Balance-previousBalance)/previousBalance)
		} else {
			if len(currentShortProfit) != 0 {
				shortPositionsProfitArr = append(shortPositionsProfitArr, currentShortProfit)
				currentShortProfit = nil
			}
		}
		previousQuantity = row.Quantity
		previousBalance = row.Balance
	}
	var longDurationArr []float64
	var shortDurationArr []float64
	var longProfitArr []float64
	var shortProfitArr []float64

	var currentLongLength float64
	var currentShortLength float64
	var longProfit float64
	var longWinRate float64
	var shortProfit float64
	var shortWinRate float64

	// Find Duration of Long Positions, based on Rebalance Interval //
	for _, value := range longPositionsArr {
		currentLongLength = float64(len(value))
		longDurationArr = append(longDurationArr, currentLongLength)
		currentLongLength = 0
	}
	averageLongDuration := utils.SumArr(longDurationArr) / float64(len(longDurationArr))
	//fmt.Println("-------------------------------")
	//fmt.Println("Total Long Positions:", len(longPositions))
	//fmt.Printf("Average Long Position Duration: %0.2f hours \n", averageLongDuration)
	algo.Stats.TotalLongPositions = len(longPositions)
	algo.Stats.AverageLongPositionDuration = averageLongDuration
	// Find long position hit rate, assumes the process of exiting position is one trade //
	for _, value := range longPositionsProfitArr {
		longProfit = utils.SumArr(value)
		longProfitArr = append(longProfitArr, longProfit)
		longProfit = 0.0
	}
	//fmt.Printf("Average Long Position Profit: %0.4f \n", utils.SumArr(longProfitArr)/float64(len(longProfitArr)))
	algo.Stats.AverageLongPositionProfit = utils.SumArr(longProfitArr) / float64(len(longProfitArr))
	// Calculate Winning and Losing Long Trades //
	winningLongTrade := make([]float64, 0)
	losingLongTrade := make([]float64, 0)
	for _, x := range longProfitArr {
		if x > 0 {
			winningLongTrade = append(winningLongTrade, x)
		} else {
			losingLongTrade = append(losingLongTrade, x)
		}
	}
	longWinRate = float64(len(winningLongTrade)) / float64(len(longPositionsProfitArr))
	averageLongWin := utils.SumArr(winningLongTrade) / float64(len(winningLongTrade))
	averageLongLoss := utils.SumArr(losingLongTrade) / float64(len(losingLongTrade))
	longRiskRewardRatio := averageLongWin / math.Abs(averageLongLoss)
	requiredLongWinRate := 1 / (1 + longRiskRewardRatio)
	// fmt.Printf("Average Long Winning Position Profit: %0.4f \n", averageLongWin)
	// fmt.Printf("Average Long Losing Position Loss: %0.4f \n", averageLongLoss)
	// fmt.Printf("Risk-Reward Ratio: 1:%0.4f \n", longRiskRewardRatio)
	// fmt.Printf("How Often Do I Have to be Right: %0.4f \n", requiredLongWinRate)
	// fmt.Printf("Total Profitable Exit Trades: %d \n", len(winningLongTrade))
	// fmt.Printf("Total Exit Trades: %d \n", len(longProfitArr))
	// fmt.Printf("Long Win Rate: %0.4f \n", longWinRate)
	algo.Stats.AverageLongWinningPositionProfit = averageLongWin
	algo.Stats.AverageLongLosingPositionLoss = averageLongLoss
	// Risk Reward value to be inputed into 1:longRiskRewardRatio //
	algo.Stats.LongRiskReward = longRiskRewardRatio
	algo.Stats.LongWinsNeeded = requiredLongWinRate
	algo.Stats.TotalLongProfitableExitTrades = len(winningLongTrade)
	algo.Stats.TotalLongExitTrades = len(longProfitArr)
	algo.Stats.LongWinRate = longWinRate
	// fmt.Println("-------------------------------")

	//Find Duration of Short Positions, Assumes Rebalance interval is hourly
	for _, value := range shortPositionsArr {
		currentShortLength = float64(len(value))
		shortDurationArr = append(shortDurationArr, currentShortLength)
		currentShortLength = 0
	}
	averageShortDuration := utils.SumArr(shortDurationArr) / float64(len(shortDurationArr))
	// fmt.Println("Total Short Positions:", len(shortPositions))
	// fmt.Printf("Average Short Position Duration: %0.2f hours \n", averageShortDuration)
	algo.Stats.TotalShortPositions = len(shortPositions)
	algo.Stats.AverageShortPositionDuration = averageShortDuration

	//Find short position hit rate, assumes the process of exiting position is one trade
	for _, value := range shortPositionsProfitArr {
		shortProfit = utils.SumArr(value)
		shortProfitArr = append(shortProfitArr, shortProfit)
		shortProfit = 0.0
	}
	// log.Println("Sum Short Position Arr", shortProfitArr)
	// fmt.Printf("Average Short Position Profit: %0.4f \n", utils.SumArr(shortProfitArr)/float64(len(shortProfitArr)))
	algo.Stats.AverageShortPositionProfit = utils.SumArr(shortProfitArr) / float64(len(shortProfitArr))
	// Calculate Winning and Losing Short Trades //
	winningShortTrade := make([]float64, 0)
	losingShortTrade := make([]float64, 0)
	for _, x := range shortProfitArr {
		if x > 0 {
			winningShortTrade = append(winningShortTrade, x)
		} else {
			losingShortTrade = append(losingShortTrade, x)
		}
	}
	shortWinRate = float64(len(winningShortTrade)) / float64(len(shortPositionsProfitArr))
	averageShortWin := utils.SumArr(winningShortTrade) / float64(len(winningShortTrade))
	averageShortLoss := utils.SumArr(losingShortTrade) / float64(len(losingShortTrade))
	shortRiskRewardRatio := averageShortWin / math.Abs(averageShortLoss)
	requiredShortWinRate := 1 / (1 + shortRiskRewardRatio)
	// fmt.Printf("Average Short Winning Position Profit: %0.4f \n", averageShortWin)
	// fmt.Printf("Average Short Losing Position Loss: %0.4f \n", averageShortLoss)
	// fmt.Printf("Risk-Reward Ratio: 1:%0.4f \n", shortRiskRewardRatio)
	// fmt.Printf("How Often Do I Have to be Right: %0.4f \n", requiredShortWinRate)
	// fmt.Printf("Total Profitable Exit Trades: %d \n", len(winningShortTrade))
	// fmt.Printf("Total Exit Trades: %d \n", len(shortProfitArr))
	// fmt.Printf("Short Win Rate: %0.4f \n", shortWinRate)
	algo.Stats.AverageShortWinningPositionProfit = averageShortWin
	algo.Stats.AverageShortLosingPositionLoss = averageShortLoss
	// Risk Reward value to be inputed into 1:shortRiskRewardRatio //
	algo.Stats.ShortRiskReward = shortRiskRewardRatio
	algo.Stats.ShortWinsNeeded = requiredShortWinRate
	algo.Stats.TotalShortProfitableExitTrades = len(winningShortTrade)
	algo.Stats.TotalShortExitTrades = len(shortProfitArr)
	algo.Stats.ShortWinRate = shortWinRate
	// fmt.Println("-------------------------------")

	// Calculate total positions metrics, irrespective of side
	totalPositionProfit := []float64{}
	totalPositionProfit = append(longProfitArr, shortProfitArr...)
	winningTrade := make([]float64, 0)
	losingTrade := make([]float64, 0)
	for _, x := range totalPositionProfit {
		if x > 0 {
			winningTrade = append(winningTrade, x)
		} else {
			losingTrade = append(losingTrade, x)
		}
	}
	averageWin := utils.SumArr(winningTrade) / float64(len(winningTrade))
	averageLoss := utils.SumArr(losingTrade) / float64(len(losingTrade))
	riskRewardRatio := averageWin / math.Abs(averageLoss)
	requiredWinRate := 1 / (1 + riskRewardRatio)
	// fmt.Printf("Total Positions: %d \n", (len(longPositions) + len(shortPositions)))
	// fmt.Printf("Total Position Average Profit: %0.4f \n", utils.SumArr(totalPositionProfit)/float64(len(totalPositionProfit)))
	// fmt.Printf("Average Winning Position Profit: %0.4f \n", averageWin)
	// fmt.Printf("Average Losing Position Loss: %0.4f \n", averageLoss)
	// fmt.Printf("Risk-Reward Ratio: 1:%0.4f \n", riskRewardRatio)
	// fmt.Printf("How Often Do I Have to be Right: %0.4f \n", requiredWinRate)
	// fmt.Printf("Total Win Rate: %0.4f \n", (float64(len(winningShortTrade)+len(winningLongTrade)))/float64((len(shortProfitArr)+len(longProfitArr))))
	algo.Stats.TotalPositions = (len(longPositions) + len(shortPositions))
	algo.Stats.AveragePositionProfit = utils.SumArr(totalPositionProfit) / float64(len(totalPositionProfit))
	algo.Stats.AverageWinningPositionProfit = averageWin
	algo.Stats.AverageLosingPositionLoss = averageLoss
	// Risk Reward value to be inputed into 1:riskRewardRatio //
	algo.Stats.TotalRiskReward = riskRewardRatio
	algo.Stats.TotalWinsNeeded = requiredWinRate
	algo.Stats.TotalWinRate = (float64(len(winningShortTrade) + len(winningLongTrade))) / float64((len(shortProfitArr) + len(longProfitArr)))

	percentDaysProfitable := utils.SumArr(profitableDays) / float64(len(profitableDays))
	// fmt.Printf("Percent Days Profitable: %0.4f \n", percentDaysProfitable)
	algo.Stats.PercentDaysProfitable = percentDaysProfitable
	return algo.Stats
}

func getMinMaxStats(history []models.History) (float64, float64, float64, float64, float64) {
	var maxProfit float64 = history[0].Profit
	var minProfit float64 = history[0].Profit

	var maxLeverage float64 = 0.0
	var minLeverage float64 = 0.0

	var maxPositionLoss float64 = 0.0
	var maxPositionProfit float64 = 0.0
	var drawdown float64 = 0.0
	var highestBalance float64 = 0.0

	for _, row := range history {
		if maxProfit < row.Profit {
			maxProfit = row.MaxProfit
			maxPositionProfit = maxProfit / row.UBalance
		}
		if minProfit > row.Profit {
			minProfit = row.MaxLoss
			maxPositionLoss = minProfit / row.UBalance
		}

		if row.UBalance > highestBalance {
			highestBalance = row.UBalance
		}

		ddDiff := utils.CalculateDifference(row.UBalance, highestBalance)
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
	return maxPositionLoss, maxPositionProfit, minLeverage, maxLeverage, drawdown
}

func logStats(algo *models.Algo, history []models.History, startTime time.Time) {
	log.Println("Start", history[0].Timestamp, "End", algo.Timestamp)
	//TODO do this during test instead of after the test
	minProfit, maxProfit, _, maxLeverage, drawdown := getMinMaxStats(history)

	historyLength := len(history)
	log.Println("historyLength", historyLength, "Start Balance", history[0].UBalance, "End Balance", history[historyLength-1].UBalance)
	percentReturn := make([]float64, historyLength)
	downsidePercentReturn := make([]float64, 0)
	last := 0.0
	for i := range history {
		if i == 0 {
			percentReturn[i] = 0
			downsidePercentReturn = append(downsidePercentReturn, 0)
		} else {
			percentReturn[i] = utils.CalculateDifference(history[i].UBalance, last)
			if math.IsNaN(percentReturn[i]) {
				percentReturn[i] = percentReturn[i-1]
			}
			downsidePercentReturn = append(downsidePercentReturn, percentReturn[i])
		}
		last = history[i].UBalance
	}

	window := 0
	if algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		window = algo.DailyInterval * 60 * 24
	} else if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		window = algo.DailyInterval * 24
	}
	var windowSharpes []float64
	for i := 0; i < len(percentReturn); i += window {
		windowReturns := make([]float64, 0)
		if (i + window) >= len(percentReturn) {
			if len(percentReturn[i:]) > 1 {
				windowReturns = percentReturn[i:]
			}
		} else {
			windowReturns = percentReturn[i : i+window]
		}
		if len(windowReturns) > 1 {
			mean, std := stat.MeanStdDev(windowReturns, nil)
			windowSharpe := mean / std
			windowSharpes = append(windowSharpes, windowSharpe)
		}
	}

	mean, std := stat.MeanStdDev(percentReturn, nil)
	_, downsideStd := stat.MeanStdDev(downsidePercentReturn, nil)
	totalSharpe := mean / std
	averageSharpe := utils.SumArr(windowSharpes) / float64(len(windowSharpes))
	sortino := mean / downsideStd
	// TODO change the scoring based on 1h / 1m
	if algo.RebalanceInterval == exchanges.RebalanceInterval().Hour {
		totalSharpe = totalSharpe * math.Sqrt(365*24)
		sortino = sortino * math.Sqrt(365*24)
		averageSharpe = averageSharpe * math.Sqrt(365*24)
	} else if algo.RebalanceInterval == exchanges.RebalanceInterval().Minute {
		totalSharpe = totalSharpe * math.Sqrt(365*24*60)
		sortino = sortino * math.Sqrt(365*24*60)
		averageSharpe = averageSharpe * math.Sqrt(365*24*60)
	}

	if math.IsNaN(totalSharpe) {
		totalSharpe = -100
	}

	if history[historyLength-1].Balance < 0 {
		totalSharpe = -100
	}

	for symbol, state := range algo.Account.MarketStates {
		if state.Info.MarketType != models.Option {
			kvparams := utils.CreateKeyValuePairs(algo.Params.GetAllParamsForSymbol(symbol), true)
			log.Printf("Balance %0.4f \n Cost %0.4f \n Quantity %0.4f \n Max Leverage %0.4f \n Max Drawdown %0.4f \n Max Profit %0.4f \n Max Position Drawdown %0.4f \n Sharpe %0.3f \n Average %d Day Sharpe %0.3f \n Sortino %0.3f \n Params: %s",
				history[historyLength-1].UBalance,
				history[historyLength-1].AverageCost,
				history[historyLength-1].Quantity,
				maxLeverage,
				drawdown,
				maxProfit,
				minProfit,
				totalSharpe,
				algo.DailyInterval,
				averageSharpe,
				sortino,
				kvparams,
			)
		}
	}

	logCloudBacktest(algo, history)

	if algo.LogBacktest {
		// Log balance history
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

	}

	algo.Result = models.Result{
		Balance:           history[historyLength-1].UBalance,
		DailyReturn:       history[historyLength-1].UBalance / float64(historyLength),
		MaxLeverage:       maxLeverage,
		MaxPositionProfit: maxProfit,
		MaxPositionDD:     minProfit,
		MaxDD:             drawdown,
		Params:            utils.CreateKeyValuePairs(algo.Params.GetAllParams(), true),
		Score:             utils.ToFixed(totalSharpe, 3),
		AverageScore:      utils.ToFixed(averageSharpe, 3),
		Sortino:           utils.ToFixed(sortino, 3),
	}

	//Log turnover stats
	if algo.LogStats == true {
		stats := getTurnoverStats(history, algo)
		statsMap := structs.Map(stats)
		kvStats := utils.CreateKeyValuePairs(statsMap, true)

		fmt.Print("Backtested Stats")
		fmt.Printf("%s", kvStats)
	}

	elapsed := time.Since(startTime)
	fmt.Println("-------------------------------")
	log.Printf("Execution Speed: %v \n", elapsed)
}

func logCloudBacktest(algo *models.Algo, history []models.History) {
	if algo.LogCloudBacktest {

		influxURL := os.Getenv("YANTRA_BACKTEST_DB_URL")
		if influxURL == "" {
			log.Fatalln("You need to set the `YANTRA_BACKTEST_DB_URL` env variable")
		}

		influxUser := os.Getenv("YANTRA_BACKTEST_DB_USER")
		influxPassword := os.Getenv("YANTRA_BACKTEST_DB_PASSWORD")

		influx, _ := client.NewHTTPClient(client.HTTPConfig{
			Addr:     influxURL,
			Username: influxUser,
			Password: influxPassword,
			Timeout:  (time.Millisecond * 1000 * 10),
		})

		// uuid := algo.Name + "-" + uuid.New().String()

		log.Println("LogCloudBacktest")
		bp, _ := client.NewBatchPoints(client.BatchPointsConfig{
			Database:  "cloudtest",
			Precision: "us",
		})

		// upsampleReturns(algo, bp, history)
		tags := map[string]string{
			"algo_name": algo.Name,
		}

		if algo.RebalanceInterval == "1m" {
			day := 1440
			lDay := 0.
			for i, row := range history {
				if i%day == 0 {
					tags["sample_format"] = "daily"
					// fmt.Println("Hour", i/(60*24), history[i].Balance)
					pct := utils.CalculateDifference(row.UBalance, lDay)
					pt, _ := client.NewPoint(
						"results",
						tags,
						map[string]interface{}{"pct_change": pct},
						row.Timestamp,
					)
					// fmt.Println(lDay, row.UBalance, pct)
					lDay = history[i].UBalance
					bp.AddPoint(pt)
				}
			}
		}
		err := client.Client.Write(influx, bp)
		log.Println(algo.Name, err)
	}
}

func upsampleReturns(algo *models.Algo, bp client.BatchPoints, history []models.History) {
	tags := map[string]string{
		"algo_name": algo.Name,
	}
	tags["currency"] = algo.Account.BaseAsset.Symbol
	if algo.RebalanceInterval == "1m" {
		day := 1440
		for i, row := range history {
			if i%day == 0 {
				tags["sample_format"] = "daily"
				pt, _ := client.NewPoint(
					"balance",
					tags,
					map[string]interface{}{"balance": row.UBalance},
					row.Timestamp,
				)
				bp.AddPoint(pt)

			}

			// if i%(day*7) == 0 {
			// 	weekly = append(weekly, models.BalanceHistory{Timestamp: row.Timestamp, UBalance: row.UBalance})
			// 	bp.AddPoint(pt)

			// }

			// if i%(day*30) == 0 {
			// 	monthly = append(monthly, models.BalanceHistory{Timestamp: row.Timestamp, UBalance: row.UBalance})
			// 	bp.AddPoint(pt)

			// }
		}
	}
	// else if algo.RebalanceInterval == "1h" {
	// 	day := 24
	// 	for i, row := range history {
	// 		if i%day == 0 {
	// 			daily = append(daily, models.BalanceHistory{Timestamp: row.Timestamp, UBalance: row.UBalance})
	// 		}

	// 		if i%(day*7) == 0 {
	// 			weekly = append(weekly, models.BalanceHistory{Timestamp: row.Timestamp, UBalance: row.UBalance})
	// 		}

	// 		if i%(day*30) == 0 {
	// 			monthly = append(monthly, models.BalanceHistory{Timestamp: row.Timestamp, UBalance: row.UBalance})
	// 		}
	// 	}
	// }
	return
}
