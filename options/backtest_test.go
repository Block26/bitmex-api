package options

import (
	"fmt"
	"testing"
)

func TestBlackScholes(t *testing.T) {
	optionType := "call"
	currentPrice := 8000.0
	strike := 10000.0
	currentTime := 1574105284
	expiry := 1577606400
	method := "blackScholes"
	impliedVol := .7
	value := GetOptionValue(optionType, currentPrice, strike, currentTime, expiry, method, impliedVol)
	fmt.Printf("Got value for option: %v [expiry %v, currentTime %v, strike %v, method %v]", value, expiry, currentTime, strike, method)
}
