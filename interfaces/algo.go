package interfaces

type Algo interface {
	Rebalance(float64)
	Connect(settingsFile string, secret bool)
	updateBalance(fillCost float64, fillAmount float64)
	CurrentProfit(price float64) float64
	RunBacktest()
}
