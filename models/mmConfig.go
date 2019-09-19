package models

type MMConfig struct {
	BaseBalance     float64
	Quantity        float64
	AverageCost     float64
	MaxOrders       int32
	Profit          float64
	EntrySpread     float64
	EntryConfidence float64
	ExitSpread      float64
	ExitConfidence  float64
	Liquidity       float64
	MaxLeverage     float64
}
