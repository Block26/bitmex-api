// Package ta provides technical analysis indicators usable in both a live and testing environment
// without the need for additional logic using github.com/markcheno/go-talib
package ta

import (
	talib "github.com/markcheno/go-talib"
	"log"
)

// GetNATR Get the Normalized Average True Range for an index and data length
func GetNATR(high []float64, low []float64, close []float64, index int, length int) []float64 {
	return talib.Natr(high[index-length-1:index], low[index-length-1:index], close[index-length-1:index], length)
}

// CreateNATREMA Create an EMA from the NATR
func CreateNATREMA(high []float64, low []float64, close []float64, index int, natrLength int, emaLength int) []float64 {
	//TODO check live
	natr := talib.Natr(high[index-natrLength-emaLength:index+1], low[index-natrLength-emaLength:index+1], close[index-natrLength-emaLength:index+1], natrLength)
	ema := CreateEMA(natr, emaLength)
	return ema
}

// GetDX Calculate a DX slice based on a data length and DX length. OHLC lengths > data length > length
func GetDX(high []float64, low []float64, close []float64, index int, datalength int, length int) []float64 {
	//TODO check live
	return talib.Dx(high[index-datalength-1:index], low[index-datalength-1:index], close[index-datalength-1:index], length)
}

// GetDX Calculate a DX slice based on a data length and DX length. OHLC lengths > data length > length
func GetRSI(close []float64, index int, dataLength int) float64 {
	//TODO check live
	rsi := talib.Rsi(close[index-dataLength-1:index], dataLength)
	return rsi[len(rsi)-1]
}

// CreateEMA Create an EMA
func CreateEMA(data []float64, length int) []float64 {
	if length <= 1 {
		log.Fatal("Length of the ema must be greater than 1")
	}
	ema := talib.Ema(data, length)
	return ema
}
