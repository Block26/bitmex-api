package models

type Bar struct {
	Timestamp string  `csv:"timestamp"`
	Open      float64 `csv:"open"`
	High      float64 `csv:"high"`
	Low       float64 `csv:"low"`
	Close     float64 `csv:"close"`
}
