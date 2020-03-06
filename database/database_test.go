package database

import (
	"fmt"
	"time"
)

func ExampleGetData() {
	start := time.Date(2018, 06, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	bars := GetData("XBTUSD", "bitmex", "1h", start, end)
	fmt.Println(len(bars), "bars retrieved from the database")
	// Output: 13153 bars retrieved from the database
}
