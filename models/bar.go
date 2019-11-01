package models

type Bar struct {
	Timestamp   string  `db:"timestamp"`
	Open        float64 `db:"open"`
	High        float64 `db:"high"`
	Low         float64 `db:"low"`
	Close       float64 `db:"close"`
	VWAP        float64 `db:"vwap"`
	Volume      float64 `db:"volume"`
	QuoteVolume float64 `db:"quote_volume"`
}
