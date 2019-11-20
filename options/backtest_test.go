package options

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/TheAlgoV2/tantradb"
)

func Arange(start, stop, step float64) []float64 {
	N := int(math.Ceil((stop - start) / step))
	arr := make([]float64, N, N)
	i := 0
	for x := start; x < stop; x += step {
		arr[i] = x
		i += 1
	}
	return arr
}

func GetNearestVol(volData []models.ImpliedVol, time int) float64 {
	vol := -1.
	for _, data := range volData {
		timeDiff := time - data.Timestamp
		if timeDiff < 0 {
			vol = data.IV
			break
		}
	}
	return vol
}

// func TestBlackScholes(t *testing.T) {
// 	testStart := time.Now().UnixNano()
// 	start := 1567756800000
// 	end := 1571990400000
// 	impliedVolData := tantradb.LoadImpliedVols("XBTUSD", start, end)
// 	calcGreeks := true
// 	numOptions := 0
// 	times := Arange(float64(start), float64(end), 3600.*1000)
// 	strikes := Arange(5000., 20000., 500.)
// 	timesToExpiry := Arange(7., 63., 7.)
// 	for _, time := range times {
// 		for _, strike := range strikes {
// 			for _, timeToExpiry := range timesToExpiry {
// 				for _, currentPrice := range strikes {
// 					for _, optionType := range []string{"call", "put"} {
// 						// fmt.Printf("Time: %v, strike: %v, timeToExpiry: %v, currentPrice: %v, optionType: %v\n", time, strike, timeToExpiry, currentPrice, optionType)
// 						impliedVol := GetNearestVol(impliedVolData, int(time))
// 						// fmt.Printf("Got nearest vol: %v\n", impliedVol)
// 						o := NewOptionTheo(optionType, currentPrice, strike, int(time), int(time+timeToExpiry), 0, impliedVol, -1)
// 						o.calcBlackScholesTheo(calcGreeks)
// 						// fmt.Printf("Got theo %v for %v option with strike %v, days to expiration %v\n", o.theo, optionType, strike, o.timeLeft*365)
// 						// if calcGreeks {
// 						// 	fmt.Printf("Delta: %v, Gamma: %v, Theta: %v, Vega: %v\n", o.delta, o.gamma, o.theta, o.vega)
// 						// }
// 						numOptions += 1
// 					}
// 				}
// 			}
// 		}
// 	}
// 	// fmt.Printf("Got implied vol data: %v\n", impliedVolData)
// 	duration := float64(time.Now().UnixNano()-testStart) / 1000000000
// 	fmt.Printf("Processed %v options in %v seconds.\n", numOptions, duration)
// }

func TestBinomialTree(t *testing.T) {
	testStart := time.Now().UnixNano()
	start := 1567756800000
	end := 1571990400000
	impliedVolData := tantradb.LoadImpliedVols("XBTUSD", start, end)
	numOptions := 0
	times := Arange(float64(start), float64(end), 10*86400.*1000)
	strikes := Arange(5000., 20000., 1000.)
	timesToExpiry := Arange(7., 63., 7.)
	for _, time := range times {
		for _, strike := range strikes {
			for _, timeToExpiry := range timesToExpiry {
				for _, currentPrice := range strikes {
					for _, optionType := range []string{"call", "put"} {
						fmt.Printf("Time: %v, strike: %v, timeToExpiry: %v, currentPrice: %v, optionType: %v\n", time, strike, timeToExpiry, currentPrice, optionType)
						impliedVol := GetNearestVol(impliedVolData, int(time))
						// fmt.Printf("Got nearest vol: %v\n", impliedVol)
						o := NewOptionTheo(optionType, currentPrice, strike, int(time), int(time+(timeToExpiry*86400)), 0, impliedVol, -1)
						o.calcBinomialTreeTheo(.5, 15)
						fmt.Printf("Got theo %v for %v option with strike %v, days to expiration %v\n", o.binomialTheo, optionType, strike, o.timeLeft*365)
						numOptions += 1
					}
				}
			}
		}
	}

	// o := NewOptionTheo("call", 8000, 10000, int(end), int(end+(10*86400000)), 0, .75, -1)
	// o.calcBinomialTreeTheo(.01, .5, 3600)
	// o.calcBinomialTreeTheo(.5, 20)

	duration := float64(time.Now().UnixNano()-testStart) / 1000000000
	fmt.Printf("Processed %v options in %v seconds.\n", numOptions, duration)
}
