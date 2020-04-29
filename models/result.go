package models

type Result struct {
	Balance           float64
	DailyReturn       float64
	MaxLeverage       float64
	MaxPositionProfit float64
	MaxPositionDD     float64
	MaxDD             float64
	Score             float64
	Params            string
}
