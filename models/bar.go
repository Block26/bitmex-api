package models

// The bar struct is meant to contain information regarding a snapshot of bar/candle data.
// We can parse raw data from exchange API responses and serialize it into Bar structs.
type Bar struct {
	Timestamp   int64   `csv:"timestamp" db:"timestamp"`       // The timestamp of the beginning of this bar
	Open        float64 `csv:"open" db:"open"`                 // The opening price of this bar
	High        float64 `csv:"high" db:"high"`                 // The highest traded price during this bar
	Low         float64 `csv:"low" db:"low"`                   // The lowest traded price during this bar
	Close       float64 `csv:"close" db:"close"`               // The ending price of this bar
	VWAP        float64 `csv:"vwap" db:"vwap"`                 // Volume-weighted average price during this bar
	Volume      float64 `csv:"volume" db:"volume"`             // Total amount traded during this bar
	QuoteVolume float64 `csv:"quote_volume" db:"quote_volume"` // Total amount traded during this var in terms of quote asset
}
