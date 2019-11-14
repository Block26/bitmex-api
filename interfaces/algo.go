package interfaces

import (
	"github.com/c-bata/goptuna"
)

type Algo interface {
	Rebalance(float64)
	Connect(settingsFile string, secret bool)
	UpdateBalance(fillCost float64, fillAmount float64)
	CurrentProfit(price float64) float64
	RunBacktest()
	Objective(trial goptuna.Trial) (float64, error)
}
