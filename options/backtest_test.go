package options

import (
	"fmt"
	"github.com/tantralabs/TheAlgoV2/tantradb"
	"testing"
	"time"
)

func TestBlackScholes(t *testing.T) {
	testStart := time.Now().Unix()
	start := 1567756800000
	end := 1571990400000
	impliedVolData := tantradb.LoadImpliedVols("XBTUSD", start, end)
	fmt.Printf("Got implied vol data: %v\n", impliedVolData)
	method := "blackScholes"
	calcGreeks := true
	for _, data := range impliedVolData {
		currentPrice := data.IndexPrice
		strike := data.Strike
		currentTime := data.Timestamp
		expiry := data.Timestamp + int(data.TimeToExpiry)
		impliedVol := data.IV
		for _, optionType := range []string{"call", "put"} {
			o := NewOptionTheo(optionType, currentPrice, strike, currentTime, expiry, 0, impliedVol, -1)
			o.calcBlackScholesTheo(calcGreeks)
			fmt.Printf("Got theo %v for %v option with strike %v, days to expiration %v [%v]\n", o.theo, optionType, strike, o.timeLeft, method)
			if calcGreeks {
				fmt.Printf("Delta: %v, Gamma: %v, Theta: %v, Vega: %v", o.delta, o.gamma, o.theta, o.vega)
			}
		}
	}
	fmt.Printf("Test took %v seconds.\n", time.Now().Unix()-testStart)
}
