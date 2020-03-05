package interfaces

import (
	"github.com/tantralabs/yantra/models"
)

type Module interface {
	SetParameters(params ...interface{})
	GetDefaultParameters() interface{}
	SetupData(bars []*models.Bar, algo Algo)
	GetLeverage(index int) float64
	GetOrderSizes(index int) (float64, float64)
	GetWeight(index int) int
}
