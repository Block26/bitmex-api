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
