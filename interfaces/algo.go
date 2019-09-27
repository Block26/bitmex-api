package interfaces

import (
	"github.com/block26/exchanges/models"
)

type Algo interface {
	rebalance(float64, float64, float64) (models.OrderArray, models.OrderArray)
}
