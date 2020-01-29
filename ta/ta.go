// Package ta provides technical analysis indicators usable in both a live and testing environment
// without the need for additional logic using github.com/markcheno/go-talib
package ta

import (
	"log"

	talib "github.com/markcheno/go-talib"
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

// GetADX Calculate an ADX slice based on a data length and DX length. OHLC lengths > data length > length
func GetADX(high []float64, low []float64, close []float64, index int, datalength int, length int) []float64 {
	//TODO check live
	return talib.Adx(high[index-datalength-1:index], low[index-datalength-1:index], close[index-datalength-1:index], length)
}

// GetMacd calculates MACD, MACDSignal, & MACDHistogram slices based on data length
func GetMacd(close []float64, inFastPeriod int, inSlowPeriod int, inSignalPeriod int, index int, datalength int) ([]float64, []float64, []float64) {
	MACD, MACDSignal, MACDHistogram := talib.Macd(close[index-datalength-1:index], inFastPeriod, inSlowPeriod, inSignalPeriod)
	return MACD, MACDSignal, MACDHistogram
}

// GetStochF calculates fastK and fastD based on HLC, fastKPeriod, fastDPeriod, and fastDMAType (always = 0)
func GetStochF(high []float64, low []float64, close []float64, fastKPeriod int, fastDPeriod int, fastDMAType talib.MaType) ([]float64, []float64) {
	return talib.StochF(high, low, close, fastKPeriod, fastDPeriod, fastDMAType)
}

// GetRoc calculates rate of change of a certain amount of hours based on close price (we use an EMA instead of close price for each hour)
func GetRoc(close []float64, inTimePeriod int) []float64 {
	roc := talib.Roc(close, inTimePeriod)
	return roc
}

// GetLinearReg calculates the linear regression of the close priced based on a given time period
func GetLinearReg(close []float64, inTimePeriod int) []float64 {
	return talib.LinearReg(close, inTimePeriod)
}

// GetDema calculates the Double EMA based on the close price and a given time period (less lag than EMA)
func GetDema(close []float64, inTimePeriod int) []float64 {
	return talib.Dema(close, inTimePeriod)
}

// GetMama calculates Mesa Adaptive Moving Average (Mama) & Following Adaptive Moving Average (Fama) using Hilbert Transform (Similar to Fourier Transform)
func GetMama(close []float64, inFastLimit float64, inSlowLimit float64) ([]float64, []float64) {
	Mama, Fama := talib.Mama(close, inFastLimit, inSlowLimit)
	return Mama, Fama
}

// GetT3 calculates Triple MA using close price, given time period, and VFactor (between 0 & 1) where 1 is the Dema and 0 is an EMA
func GetT3(close []float64, inTimePeriod int, inVFactor float64) []float64 {
	return talib.T3(close, inTimePeriod, inVFactor)
}

// GetKama Create a Kama
func GetKAMA(close []float64, length int) []float64 {
	if length <= 1 {
		log.Fatal("Length of the ema must be greater than 1")
	}
	kama := talib.Kama(close, length)
	return kama
}

// GetHTTrendline Create a Trendline
func GetHTTrendline(close []float64) []float64 {
	trendline := talib.HtTrendline(close)
	return trendline
}

// GetRsi calculates the RSI for a given time period. Scales from 0-100 where 70 usually signals overbought market and 30 signal oversold market
func GetRsi(close []float64, inTimePeriod int) []float64 {
	return talib.Rsi(close, inTimePeriod)
}

// GetUltOsc measures price momentum, similar to RSI, except uses 3 different time signals (7, 14, & 28). Above 70 is overbought, and below 30 is oversold (typically).
func GetUltOsc(high []float64, low []float64, close []float64, inTimePeriod1 int, inTimePeriod2 int, inTimePeriod3 int) []float64 {
	UltOsc := talib.UltOsc(high, low, close, inTimePeriod1, inTimePeriod2, inTimePeriod3)
	return UltOsc
}

// GetBBands calculates an upper band, middle band, and lower band based on a given time period, upper STD, & lower STD (is the same as the upper but talib makes it negative for you)
func GetBBands(close []float64, inTimePeriod int, inNbDevUp float64, inNbDevDn float64, inMAType talib.MaType) ([]float64, []float64, []float64) {
	UpperBand, MiddleBand, LowerBand := talib.BBands(close, inTimePeriod, inNbDevUp, inNbDevDn, inMAType)
	return UpperBand, MiddleBand, LowerBand
}

// GetTrima calculates triangular moving average, which is similar to an MA but is averaged twice. It is given a time period.
func GetTrima(close []float64, inTimePeriod int) []float64 {
	return talib.Trima(close, inTimePeriod)
}

// Get Wma calculates a weighted moving average given a time period, giving more weight to more recent data as opposed to older data
func GetWma(close []float64, inTimePeriod int) []float64 {
	return talib.Wma(close, inTimePeriod)
}

// CreateEMA Create an EMA
func CreateEMA(close []float64, length int) []float64 {
	if length <= 1 {
		log.Fatal("Length of the ema must be greater than 1")
	}
	ema := talib.Ema(close, length)
	return ema
}

//HeikinashiCandles - from candle values extracts heikinashi candle values.
func GetHeikinashiCandles(highs []float64, opens []float64, closes []float64, lows []float64) ([]float64, []float64, []float64, []float64) {
	return talib.HeikinashiCandles(highs, opens, closes, lows)
}
