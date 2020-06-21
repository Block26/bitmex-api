package models

import (
	"log"
	"math"
	"sort"
	"time"

	"github.com/tantralabs/tradeapi/iex"
)

// The Data struct includes historical bar data for a given asset. It has the ability to configure and display
// the data at arbitrary intervals/sampling frequencies.
type Data struct {
	minuteBars     []*Bar
	lastIndex      time.Time
	index          int
	data           map[int]OHLCV
	lookbackLength int
	isTest         bool
}

const (
	hour int64 = 3600000
)

// SetupDataModel is always passed minute level data.
func SetupDataModel(minuteBars []*Bar, initialIndex int, isTest bool) Data {
	// Sort the data where index 0 is the start and index -1 is the end
	sort.Slice(minuteBars, func(i, j int) bool { return minuteBars[i].Timestamp < minuteBars[j].Timestamp })
	// Remove from the start of the dataset and pin to the start of an hour
	// this is done so that when we compute 3m,5m, etc they all have the same staple eg 1hour
	// instead of having a 15m bar close 16m into the Hour
	firstHourIndex := 0
	for i := 0; i <= 60; i++ {
		if minuteBars[i].Timestamp%hour == 0 {
			firstHourIndex = i
			break
		}
	}

	minuteBars = minuteBars[firstHourIndex : len(minuteBars)-1]
	return Data{
		index:      initialIndex - firstHourIndex,
		minuteBars: minuteBars,
		data:       make(map[int]OHLCV),
		isTest:     isTest,
	}
}

// Return minute bar data as a slice of Bar structs.
func (d *Data) GetBarData() []*Bar {
	return d.minuteBars
}

// Serialize raw TradeBin data into the data model.
func (d *Data) AddDataFromTradeBin(tradeBin iex.TradeBin) {
	arr := make([]*Bar, 0)

	bar := &Bar{
		Timestamp:   tradeBin.Timestamp.Unix() * 1000,
		Open:        tradeBin.Open,
		High:        tradeBin.High,
		Low:         tradeBin.Low,
		Close:       tradeBin.Close,
		Volume:      tradeBin.Volume,
		QuoteVolume: tradeBin.Volume * tradeBin.Close,
	}
	arr = append(arr, bar)

	d.AddData(arr)
}

// Update the new index of this data (usually called when we receive new data from the exchange API).
func (d *Data) IncrementIndex() {
	d.index += 1
}

// Add new data to the dataset.
func (d *Data) AddData(newBars []*Bar) {
	// Sort the new data
	sort.Slice(newBars, func(i, j int) bool { return newBars[i].Timestamp < newBars[j].Timestamp })

	newDataLength := len(newBars)
	timestamps := make([]int64, newDataLength)
	for i := 0; i < newDataLength; i++ {
		timestamps[i] = d.minuteBars[len(d.minuteBars)-newDataLength+i].Timestamp
	}

	d.lookbackLength = 0
	if newBars != nil {
		// log.Println("Adding", len(newBars), "new bars")
		for y := range newBars {
			if !containsInt(timestamps, newBars[y].Timestamp) {
				d.minuteBars = append(d.minuteBars, &Bar{
					Timestamp: newBars[y].Timestamp,
					Open:      newBars[y].Open,
					High:      newBars[y].High,
					Low:       newBars[y].Low,
					Close:     newBars[y].Close,
				})
				d.lookbackLength += 1
				d.IncrementIndex()
			}
		}
	}

	for resampleInterval, _ := range d.data {
		// TODO Make this function work live
		// d.rebuildOHLCV(resampleInterval)
		delete(d.data, resampleInterval)
	}
}

// Get OHLCV data from the data model with a given resampling interval.
func (d *Data) GetOHLCVData(resampleInterval int) (data OHLCV, index int) {
	data = d.getOHLCV(resampleInterval)
	index = len(data.Timestamp) - 1
	// logger.Debug("Last Timestamp for", resampleInterval, "min", time.Unix(data.Timestamp[index]/1000, 0).UTC())
	// logger.Debug("2nd to Last Timestamp for", resampleInterval, "min", time.Unix(data.Timestamp[index-1]/1000, 0).UTC())
	return
}

// Get all data with a given resample interval.
func (d *Data) FetchAllData(resampleInterval int) (data OHLCV) {
	data = d.getOHLCV(resampleInterval, true)
	return
}

// Get the current index in the data model.
func (d *Data) GetCurrentIndex(resampleInterval int) int {
	return int(d.index / resampleInterval)
}

// Get all minute data as a single OHLCV struct.
func (d *Data) GetMinuteData() OHLCV {
	return d.getOHLCV(1)
}

// getOHLCVBars Break down the bars into open, high, low, close arrays that are easier to manipulate.
func (d *Data) getOHLCV(resampleInterval int, all ...bool) OHLCV {
	bars := d.minuteBars
	adjuster := 0
	resampledIndex := int(math.Ceil(float64(d.index)/float64(resampleInterval))) - 1
	if float64(resampledIndex) == float64(d.index)/float64(resampleInterval) || resampleInterval == 1 {
		adjuster = 1
	}

	length := resampledIndex
	if d.isTest {
		length = (len(bars) / resampleInterval) + 1
	}
	// Check to see if we have already cached the data
	if val, ok := d.data[resampleInterval]; ok {
		if len(all) > 0 && all[0] == true {
			return OHLCV{
				Timestamp: val.Timestamp,
				Open:      val.Open,
				High:      val.High,
				Low:       val.Low,
				Close:     val.Close,
				Volume:    val.Volume,
			}
		}
		last := resampledIndex - adjuster
		return OHLCV{
			Timestamp: val.Timestamp[:last],
			Open:      val.Open[:last],
			High:      val.High[:last],
			Low:       val.Low[:last],
			Close:     val.Close[:last],
			Volume:    val.Volume[:last],
		}
	} else {
		// We want to preprocess all the data and cache it for the test to use and index later
		if resampleInterval == 1 {
			length = len(bars)
		}
		ohlcv := OHLCV{
			Timestamp: make([]int64, length),
			Open:      make([]float64, length),
			High:      make([]float64, length),
			Low:       make([]float64, length),
			Close:     make([]float64, length),
			Volume:    make([]float64, length),
		}
		if resampleInterval == 1 {
			for i := 0; i < length; i++ {
				ohlcv.Open[i] = bars[i].Open
				ohlcv.Close[i] = bars[i].Close
				ohlcv.Timestamp[i] = bars[i].Timestamp
				ohlcv.High[i] = bars[i].High
				ohlcv.Low[i] = bars[i].Low
				ohlcv.Volume[i] = bars[i].Volume
			}
			d.data[resampleInterval] = ohlcv
		} else {
			for i := 0; i < length-adjuster; i++ {
				oldIndex := resampleInterval * (i + 1)
				if oldIndex >= len(bars) {
					break
				}
				ohlcv.Open[i] = bars[oldIndex-resampleInterval].Open
				ohlcv.Close[i] = bars[oldIndex].Close
				ohlcv.Timestamp[i] = bars[oldIndex].Timestamp
				low := ohlcv.Open[i]

				var high, volume float64
				for j := -resampleInterval; j <= 0; j++ {
					if high < bars[oldIndex+j].High {
						high = bars[oldIndex+j].High
					}

					if low > bars[oldIndex+j].Low {
						low = bars[oldIndex+j].Low
					}
					volume += bars[oldIndex+j].Volume
				}

				ohlcv.High[i] = high
				ohlcv.Low[i] = low
				ohlcv.Volume[i] = volume
			}
			d.data[resampleInterval] = ohlcv
		}
	}

	if len(all) > 0 && all[0] == true {
		return OHLCV{
			Timestamp: d.data[resampleInterval].Timestamp,
			Open:      d.data[resampleInterval].Open,
			High:      d.data[resampleInterval].High,
			Low:       d.data[resampleInterval].Low,
			Close:     d.data[resampleInterval].Close,
			Volume:    d.data[resampleInterval].Volume,
		}
	}

	// fmt.Println("adjuster", adjuster, "resampledIndex", resampledIndex, "length", length)
	// golang is wierd and so if you use : to select from a slice it will always leave out the last element
	// so we check if we want the last element here and return the whole array if so
	if len(d.data[resampleInterval].Timestamp) == length {
		return OHLCV{
			Timestamp: d.data[resampleInterval].Timestamp,
			Open:      d.data[resampleInterval].Open,
			High:      d.data[resampleInterval].High,
			Low:       d.data[resampleInterval].Low,
			Close:     d.data[resampleInterval].Close,
			Volume:    d.data[resampleInterval].Volume,
		}
	} else {
		return OHLCV{
			Timestamp: d.data[resampleInterval].Timestamp[:resampledIndex-adjuster],
			Open:      d.data[resampleInterval].Open[:resampledIndex-adjuster],
			High:      d.data[resampleInterval].High[:resampledIndex-adjuster],
			Low:       d.data[resampleInterval].Low[:resampledIndex-adjuster],
			Close:     d.data[resampleInterval].Close[:resampledIndex-adjuster],
			Volume:    d.data[resampleInterval].Volume[:resampledIndex-adjuster],
		}
	}

}

// Reconstruct the underlying OHLCV data with a given resample interval.
func (d *Data) rebuildOHLCV(resampleInterval int) {
	bars := d.minuteBars
	if _, ok := d.data[resampleInterval]; !ok {
		log.Fatalln("Trying to rebuild a dataset that no longer exists", resampleInterval)
	} else {
		startIndex := len(d.data[resampleInterval].Timestamp) - 1
		length := d.lookbackLength / resampleInterval
		log.Println(length, "new", resampleInterval, "x min added")
		ohlcv := d.data[resampleInterval]
		adjuster := 0
		if resampleInterval == 1 {
			adjuster = 1
		}
		for i := 1; i <= length; i++ {
			index := (startIndex * resampleInterval) + (i * resampleInterval)

			ohlcv.Open = append(ohlcv.Open, bars[index-resampleInterval-adjuster].Open)
			ohlcv.Close = append(ohlcv.Close, bars[index-adjuster].Close)
			ohlcv.Timestamp = append(ohlcv.Timestamp, bars[index-adjuster].Timestamp)

			var high, low, volume float64
			for k := -resampleInterval; k < 0; k++ {
				if high < bars[index+k].High {
					high = bars[index+k].High
				}

				if low > bars[index+k].Low {
					low = bars[index+k].Low

				}
				volume += bars[index+k].Volume
			}

			ohlcv.High = append(ohlcv.High, high)
			ohlcv.Low = append(ohlcv.Low, low)
			ohlcv.Volume = append(ohlcv.Volume, volume)
		}
		d.data[resampleInterval] = ohlcv
	}
}

// Determine whether a slice of ints contains a certain value (TODO: this should be a utils function).
func containsInt(s []int64, e int64) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
