package database

import "time"

import "fmt"

import "github.com/tantralabs/yantra/exchanges"

func ExampleGetData() {
	start := time.Date(2018, 06, 01, 0, 0, 0, 0, time.UTC)
	end := time.Date(2019, 12, 01, 0, 0, 0, 0, time.UTC)
	bars := GetData("XBTUSD", exchanges.Bitmex, exchanges.RebalanceInterval().Hour, start, end)
	fmt.Println(len(bars), "bars retrieved from the database")
	// Output: 13153 bars retrieved from the database
}
