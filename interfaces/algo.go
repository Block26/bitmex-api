package interfaces

import (
	"github.com/c-bata/goptuna"
	"github.com/tantralabs/exchanges/models"
)

type Algo interface {
	Rebalance(float64, float64, float64) (models.OrderArray, models.OrderArray)
	Connect(settingsFile string, secret bool)
	UpdateBalance(fillCost float64, fillAmount float64)
	CurrentProfit(price float64) float64
	RunBacktest()
	Objective(trial goptuna.Trial) (float64, error)
}
