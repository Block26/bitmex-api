// Package ta provides technical analysis indicators usable in both a live and testing environment
// without the need for additional logic using github.com/markcheno/go-talib
package yantra

import (
	"log"

	talib "github.com/markcheno/go-talib"
	"github.com/tantralabs/yantra/models"
)

// GetNATR Get the Normalized Average True Range for an index and data length
func GetNATR(ohlcv models.OHLCV, index int, datalength int, length int) []float64 {
	return talib.Natr(ohlcv.High[index-datalength:index], ohlcv.Low[index-datalength:index], ohlcv.Close[index-datalength:index], length)
}

// GetNATR Get the Average True Range for an index and data length
func GetATR(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	data := ms.OHLCV.FetchAllData(dataInterval)
	return talib.Atr(data.High, data.Low, data.Close, inTimePeriod)
}

// GetNATR Get the Normalized Average True Range for an index and data length
func GetNATRVariableLength(ohlcv models.OHLCV, index int, datalength int, natrLength int) []float64 {
	return talib.Natr(ohlcv.High[index-datalength:index], ohlcv.Low[index-datalength:index], ohlcv.Close[index-datalength:index], natrLength)
}

// CreateNATREMA Create an EMA from the NATR
func CreateNATREMA(high []float64, low []float64, close []float64, index int, natrLength int, emaLength int) []float64 {
	//TODO check live
	natr := talib.Natr(high[index-natrLength-emaLength:index+1], low[index-natrLength-emaLength:index+1], close[index-natrLength-emaLength:index+1], natrLength)
	ema := talib.Ema(natr, emaLength)
	return ema
}

// GetDX Calculate a DX slice based on a data length and DX length. OHLC lengths > data length > length
func GetDX(ohlcv models.OHLCV, index int, datalength int, length int) []float64 {
	//TODO check live
	return talib.Dx(ohlcv.High[index-datalength:index], ohlcv.Low[index-datalength:index], ohlcv.Close[index-datalength:index], length)
}

// GetADX Calculate an ADX slice based on a data length and DX length. OHLC lengths > data length > length
func GetADX(ohlcv models.OHLCV, index int, datalength int, length int) []float64 {
	//TODO check live
	return talib.Dx(ohlcv.High[index-datalength:index], ohlcv.Low[index-datalength:index], ohlcv.Close[index-datalength:index], length)
}

// GetMacd calculates MACD, MACDSignal, & MACDHistogram slices based on data length
func GetMacd(ohlcv models.OHLCV, inFastPeriod int, inSlowPeriod int, inSignalPeriod int, index int, datalength int) ([]float64, []float64, []float64) {
	MACD, MACDSignal, MACDHistogram := talib.Macd(ohlcv.Close[index-datalength-1:index], inFastPeriod, inSlowPeriod, inSignalPeriod)
	return MACD, MACDSignal, MACDHistogram
}

// GetStochF calculates fastK and fastD based on HLC, fastKPeriod, fastDPeriod, and fastDMAType (always = 0)
func GetStochF(ms models.MarketState, dataInterval int, fastKPeriod int, fastDPeriod int, fastDMAType talib.MaType) ([]float64, []float64) {
	data := ms.OHLCV.FetchAllData(dataInterval)
	return talib.StochF(data.High, data.Low, data.Close, fastKPeriod, fastDPeriod, fastDMAType)
}

// GetRoc calculates rate of change of a certain amount of hours based on close price (we use an EMA instead of close price for each hour)
func GetRoc(ms models.MarketState, ma []float64, length int) []float64 {
	// close := ms.OHLCV.FetchAllData(dataInterval).Close
	roc := talib.Roc(ma, length)
	return roc
}

// GetLinearReg calculates the linear regression of the close priced based on a given time period
func GetLinearReg(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.LinearReg(close, inTimePeriod)
}

// GetDema calculates the Double EMA based on the close price and a given time period (less lag than EMA)
func GetDema(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.Dema(close, inTimePeriod)
}

// GetMama calculates Mesa Adaptive Moving Average (Mama) & Following Adaptive Moving Average (Fama) using Hilbert Transform (Similar to Fourier Transform)
func GetMama(ms models.MarketState, dataInterval int, inFastLimit float64, inSlowLimit float64) ([]float64, []float64) {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	Mama, Fama := talib.Mama(close, inFastLimit, inSlowLimit)
	return Mama, Fama
}

// GetT3 calculates Triple MA using close price, given time period, and VFactor (between 0 & 1) where 1 is the Dema and 0 is an EMA
func GetT3(ms models.MarketState, dataInterval int, inTimePeriod int, inVFactor float64) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.T3(close, inTimePeriod, inVFactor)
}

// GetKama Create a Kama
func GetKAMA(ms models.MarketState, dataInterval int, length int) []float64 {
	if length <= 1 {
		log.Fatal("Length of the ema must be greater than 1")
	}
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	kama := talib.Kama(close, length)
	return kama
}

// GetHTTrendline Create a Trendline
func GetHTTrendline(ms models.MarketState, dataInterval int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	trendline := talib.HtTrendline(close)
	return trendline
}

// GetRsi calculates the RSI for a given time period. Scales from 0-100 where 70 usually signals overbought market and 30 signal oversold market
func GetRsi(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	rsi := talib.Rsi(close, inTimePeriod)
	return rsi
}

// GetUltOsc measures price momentum, similar to RSI, except uses 3 different time signals (7, 14, & 28). Above 70 is overbought, and below 30 is oversold (typically).
func GetUltOsc(ms models.MarketState, dataInterval int, inTimePeriod1 int, inTimePeriod2 int, inTimePeriod3 int) []float64 {
	data := ms.OHLCV.FetchAllData(dataInterval)
	UltOsc := talib.UltOsc(data.High, data.Low, data.Close, inTimePeriod1, inTimePeriod2, inTimePeriod3)
	return UltOsc
}

// GetBBands calculates an upper band, middle band, and lower band based on a given time period, upper STD, & lower STD (is the same as the upper but talib makes it negative for you)
func GetBBands(ms models.MarketState, dataInterval int, inTimePeriod int, inNbDevUp float64, inNbDevDn float64, inMAType talib.MaType) ([]float64, []float64, []float64) {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	UpperBand, MiddleBand, LowerBand := talib.BBands(close, inTimePeriod, inNbDevUp, inNbDevDn, inMAType)
	return UpperBand, MiddleBand, LowerBand
}

// GetTrima calculates triangular moving average, which is similar to an MA but is averaged twice. It is given a time period.
func GetTrima(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.Trima(close, inTimePeriod)
}

// Get Wma calculates a weighted moving average given a time period, giving more weight to more recent data as opposed to older data
func GetWma(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.Wma(close, inTimePeriod)
}

// CreateEMA Create an EMA
func CreateEMA(ms models.MarketState, dataInterval int, length int) []float64 {
	if length <= 1 {
		log.Fatal("Length of the ema must be greater than 1")
	}
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	ema := talib.Ema(close, length)
	return ema
}

//HeikinashiCandles - from candle values extracts heikinashi candle values.
func GetHeikinashiCandles(ms models.MarketState, dataInterval int) ([]float64, []float64, []float64, []float64) {
	data := ms.OHLCV.FetchAllData(dataInterval)
	return talib.HeikinashiCandles(data.High, data.Open, data.Close, data.Low)
}

// GetSar calculates the parabolic sar (stop and reverse)
func GetSar(ms models.MarketState, dataInterval int, inAcceleration float64, inMaximum float64) []float64 {
	data := ms.OHLCV.FetchAllData(dataInterval)
	return talib.Sar(data.High, data.Low, inAcceleration, inMaximum)
}

// GetSma calculates the Simple Moving Average based on the close price and a given time period.
func GetSma(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.Sma(close, inTimePeriod)
}

//Momentum measures the speed the price changes.
func GetMom(ms models.MarketState, dataInterval int, inTimePeriod int) []float64 {
	close := ms.OHLCV.FetchAllData(dataInterval).Close
	return talib.Mom(close, inTimePeriod)
}

//Money Flow - calculates money flow index (works similar to RSI but incorporates volume)
func GetMfi(ms models.MarketState, dataInterval int, TimePeriod int) []float64 {
	data := ms.OHLCV.FetchAllData(dataInterval)
	return talib.Mfi(data.High, data.Low, data.Close, data.Volume, TimePeriod)
}

//Commodity Channel Index measures the difference between the current price and the historical average price. (can be positive and negative)
func GetCci(ms models.MarketState, dataInterval int, TimePeriod int) []float64 {
	data := ms.OHLCV.FetchAllData(dataInterval)
	return talib.Cci(data.High, data.Low, data.Close, TimePeriod)
}
