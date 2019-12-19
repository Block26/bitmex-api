package algo

import (
	ta "github.com/d4l3k/talib"
)

func GetNATR(high []float64, low []float64, close []float64, index int32, length int32) []float64 {
	//TODO check live
	return ta.Natr(high[index-length-1:index], low[index-length-1:index], close[index-length-1:index], length)
}

func CreateNATREMA(high []float64, low []float64, close []float64, index int32, natrLength int32, emaLength int32) []float64 {
	//TODO check live
	natr := ta.Natr(high[index-natrLength-emaLength:index+1], low[index-natrLength-emaLength:index+1], close[index-natrLength-emaLength:index+1], natrLength)
	ema := CreateEMA(natr, emaLength)
	return ema
}

func GetDX(high []float64, low []float64, close []float64, index int32, length int32) []float64 {
	//TODO check live
	return ta.Dx(high[index-length-1:index], low[index-length-1:index], close[index-length-1:index], length)
}

func CreateEMA(data []float64, length int32) []float64 {
	newEma := make([]float64, len(data))
	ema := ta.Ema(data, length)
	for x := range newEma {
		if x < int(length) {
			newEma[x] = 0
		} else {
			newEma[x] = ema[x-int(length)]
		}
	}
	return newEma
}
