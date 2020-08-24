package models

import (
	"os"

	"github.com/gocarina/gocsv"
)

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

// ExportBars writes bar data to a new csv file
func ExportBars(data []*Bar, file string, di int) error {
	// Resample the data if we need to ...
	if di > 1 {
		data = ResampleBars(data, di)
	}

	// Open file
	f, err := os.Create(file)
	if err != nil {
		return err
	}

	if err := gocsv.MarshalFile(&data, f); err != nil {
		return err
	}

	f.Close()

	return nil
}

func ResampleBars(bars []*Bar, resampleInterval int) []*Bar {
	dataModel := SetupDataModel(bars, 0, true) // TODO: params.CSVExportStartIndex for initialIndex?
	ohlcv, _ := dataModel.GetOHLCVData(resampleInterval)
	return ohlcv.ToBars()
}
