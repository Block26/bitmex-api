package models

import (
	"log"
	"sort"
	"time"
)

type Data struct {
	minuteBars     []*Bar
	lastIndex      time.Time
	data           map[int]OHLCV
	lookbackLength int
}

const (
	hour int64 = 3600000
)

// SetupDataModel is always passed minute level data
func SetupDataModel(minuteBars []*Bar) Data {
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

	return Data{
		minuteBars: minuteBars[firstHourIndex : len(minuteBars)-1],
		data:       make(map[int]OHLCV),
	}
}

func (d *Data) GetBarData() []*Bar {
	return d.minuteBars
}

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
			}
		}
	}

	for resampleInterval, _ := range d.data {
		d.rebuildOHLCV(resampleInterval)
	}
}

func (d *Data) GetMinuteData(resampleInterval ...int) OHLCV {
	if resampleInterval != nil {
		return d.getOHLCV(resampleInterval[0])
	} else {
		return d.getOHLCV(1)
	}
}

func (d *Data) GetHourData() OHLCV {
	return d.getOHLCV(60)
}

func (d *Data) GetFiveMinuteData() OHLCV {
	return d.getOHLCV(5)
}

// getOHLCVBars Break down the bars into open, high, low, close arrays that are easier to manipulate.
func (d *Data) getOHLCV(resampleInterval int) OHLCV {
	bars := d.minuteBars

	if val, ok := d.data[resampleInterval]; ok {
		return val
	} else {
		length := (len(bars) / resampleInterval) + 1
		ohlcv := OHLCV{
			Timestamp: make([]int64, length),
			Open:      make([]float64, length),
			High:      make([]float64, length),
			Low:       make([]float64, length),
			Close:     make([]float64, length),
			Volume:    make([]float64, length),
		}

		for i := 1; i < length; i++ {
			oldIndex := (i * resampleInterval)
			ohlcv.Open[i] = bars[oldIndex-resampleInterval].Open
			ohlcv.Close[i] = bars[oldIndex-1].Close
			ohlcv.Timestamp[i] = bars[oldIndex-1].Timestamp

			var high, low, volume float64
			for j := -resampleInterval; j < 0; j++ {
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

	return d.data[resampleInterval]
}

func (d *Data) rebuildOHLCV(resampleInterval int) {
	bars := d.minuteBars
	if _, ok := d.data[resampleInterval]; !ok {
		log.Fatalln("Trying to rebuild a dataset that no longer exists", resampleInterval)
	} else {
		startIndex := len(d.data[resampleInterval].Timestamp) - 1
		length := d.lookbackLength / resampleInterval
		if resampleInterval == 1 {
			length -= 1
		}
		log.Println(length, "new", resampleInterval, "x min added")
		ohlcv := d.data[resampleInterval]
		for i := 1; i <= length; i++ {
			index := (startIndex * resampleInterval) + (i * resampleInterval)

			ohlcv.Open = append(ohlcv.Open, bars[index-resampleInterval].Open)
			ohlcv.Close = append(ohlcv.Close, bars[index].Close)
			ohlcv.Timestamp = append(ohlcv.Timestamp, bars[index].Timestamp)

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

func containsInt(s []int64, e int64) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}