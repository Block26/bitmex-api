package module

import (
	"github.com/tantralabs/yantra/models"
)

const ModuleName = "MY-MOD"

// SetParameters should be used to set custom parameters
func SetParameters(algo *models.Algo, p Params, symbol string) {
	algo.Params.Add(symbol, ModuleName, p)
}

// GetDefaultParameters should return a basic parameter set for your module
func GetDefaultParameters() Params {
	return Params{}
}

// SetupData is called at the start of the algo and is used to setup intial TA signals and other data processing
func SetupData(algo *models.Algo) {
}

// GetWeight should return 0, 1, -1 depending on if you want to be neutral, long, or short.
func GetWeight(index int) int {
	return 0
}

// GetLeverage should return a float between 0 and 1
// if you set leverage to 0 and you are still in a position yantra will stop placing orders.
func GetLeverage(index int) float64 {
	return 1.0
}
