package models

type History struct {
	Timestamp   string
	Balance     float64
	Quantity    float64
	AverageCost float64
	Leverage    float64
	Profit      float64
	Price       float64
}

type BalanceHistory struct {
	Timestamp string  `csv:"timestamp"`
	Balance   float64 `csv:"balance"`
}
