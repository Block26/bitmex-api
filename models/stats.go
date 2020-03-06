package models

type Stats struct {
	TotalLongPositions                int
	TotalShortPositions               int
	TotalPositions                    int
	AverageLongPositionDuration       float64
	AverageShortPositionDuration      float64
	AverageLongPositionProfit         float64
	AverageShortPositionProfit        float64
	AveragePositionProfit             float64
	AverageLongWinningPositionProfit  float64
	AverageShortWinningPositionProfit float64
	AverageWinningPositionProfit      float64
	AverageLongLosingPositionLoss     float64
	AverageShortLosingPositionLoss    float64
	AverageLosingPositionLoss         float64
	LongRiskReward                    float64
	ShortRiskReward                   float64
	TotalRiskReward                   float64
	LongWinsNeeded                    float64
	ShortWinsNeeded                   float64
	TotalWinsNeeded                   float64
	TotalLongProfitableExitTrades     int
	TotalLongExitTrades               int
	TotalShortProfitableExitTrades    int
	TotalShortExitTrades              int
	LongWinRate                       float64
	ShortWinRate                      float64
	TotalWinRate                      float64
	AverageDailyWeightChanges         float64
	PercentDaysProfitable             float64
}
