package algo

import (
	"log"
	ta "github.com/markcheno/go-talib"
)

func GetNATR(high []float64, low []float64, close []float64, index int, length int) []float64 {
	//TODO check live
	return ta.Natr(high[index-length-1:index], low[index-length-1:index], close[index-length-1:index], length)
}

func CreateNATREMA(high []float64, low []float64, close []float64, index int, natrLength int, emaLength int) []float64 {
	//TODO check live
	natr := ta.Natr(high[index-natrLength-emaLength:index+1], low[index-natrLength-emaLength:index+1], close[index-natrLength-emaLength:index+1], natrLength)
	ema := CreateEMA(natr, emaLength)
	return ema
}

func GetDX(high []float64, low []float64, close []float64, index int, length int) []float64 {
	//TODO check live
	return ta.Dx(high[index-length-1:index], low[index-length-1:index], close[index-length-1:index], length)
}

func CreateEMA(data []float64, length int) []float64 {
	if length <= 1 {
		log.Fatal("Length of the ema must be greater than 1")
	}
	// newEma := make([]float64, len(data))
	ema := ta.Ema(data, length)
	// for x := range newEma {
	// 	if x < int(length) {
	// 		newEma[x] = 0
	// 	} else {
	// 		newEma[x] = ema[x-int(length)]
	// 	}
	// }
	return ema
}
