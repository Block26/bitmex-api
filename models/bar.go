package models

type Bar struct {
	Timestamp   int64   `csv:"timestamp" db:"timestamp"`
	Open        float64 `csv:"open" db:"open"`
	High        float64 `csv:"high" db:"high"`
	Low         float64 `csv:"low" db:"low"`
	Close       float64 `csv:"close" db:"close"`
	VWAP        float64 `csv:"vwap" db:"vwap"`
	Volume      float64 `csv:"volume" db:"volume"`
	QuoteVolume float64 `csv:"quote_volume" db:"quote_volume"`
}
