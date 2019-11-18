package options

import (
	"fmt"
	"github.com/tantralabs/TheAlgoV2/tantradb"
	"testing"
	"time"
)

func TestBlackScholes(t *testing.T) {
	testStart := time.Now().Unix()
	start := 1546644871291
	end := 1568365292609
	impliedVolData := tantradb.LoadImpliedVols("XBTUSD", start, end)
	fmt.Printf("Loaded implied vol data: %v", impliedVolData)
	method := "blackScholes"
	for _, data := range impliedVolData {
		currentPrice := data.IndexPrice
		strike := data.Strike
		currentTime := data.Timestamp / 1000
		expiry := (data.Timestamp + int(data.TimeToExpiry)) / 1000
		impliedVol := data.IV
		for _, optionType := range []string{"call", "put"} {
			value := GetOptionValue(optionType, currentPrice, strike, currentTime, expiry, method, impliedVol)
			fmt.Printf("Got theo %v for %v option with strike %v, time to expiration %v [%v]\n", value, optionType, strike, expiry-currentTime, method)
		}
	}
	fmt.Printf("Test took %v seconds.\n", time.Now().Unix()-testStart)
}
