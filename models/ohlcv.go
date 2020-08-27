package models

// Represents concise Open, High, Low, Close, and Volume data in a single struct.
type OHLCV struct {
	Timestamp []int64
	Open      []float64
	High      []float64
	Low       []float64
	Close     []float64
	Volume    []float64
}

// ToBars returns the ohlcv data as a list of bars
func (ohlcv *OHLCV) ToBars() []*Bar {
	n := len(ohlcv.Timestamp)
	bars := make([]*Bar, n)
	nEmptyBars := 0
	for i := 0; i < n; i++ {
		if ohlcv.Timestamp[i] == 0 {
			nEmptyBars++
		} else {
			bars[i] = &Bar{
				Timestamp: ohlcv.Timestamp[i],
				Open:      ohlcv.Open[i],
				High:      ohlcv.High[i],
				Low:       ohlcv.Low[i],
				Close:     ohlcv.Close[i],
				Volume:    ohlcv.Volume[i],
			}
		}
	}
	return bars[:len(bars)-nEmptyBars]
}
