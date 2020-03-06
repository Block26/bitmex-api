package models

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/chobie/go-gaussian"
)

const PI float64 = 3.14159265359
const Day = 86400
const DefaultVolatility = .6

type OptionModel int

const (
	BlackScholes OptionModel = iota
	BinomialTree
)

var optionModels = [...]string{
	"BlackScholes",
	"BinomialTree",
}

func (optionModel OptionModel) String() string { return optionModels[optionModel] }

type OptionTheo struct {
	Strike                  float64     // Strike price
	UnderlyingPrice         *float64    // Underlying price
	InterestRate            float64     // Risk free rate (assume 0)
	Volatility              float64     // Implied volatility
	Expiry                  int         // Expiration date (ms)
	TimeLeft                float64     // Time left until expiry (years)
	OptionType              string      // "call" or "put"
	Theo                    float64     // Theoretical value of the option according to proprietary model
	Delta                   float64     // Change in theo wrt. 1 USD change in UnderlyingPrice
	Theta                   float64     // Change in theo wrt. 1 day decrease in timeLeft
	Gamma                   float64     // Change in delta wrt. 1 USD change in UnderlyingPrice
	Vega                    float64     // Change in theo wrt. 1% increase in volatility
	WeightedVega            float64     // Vega / Vega of ATM option
	DenominatedInUnderlying bool        // True if option price is quoted in terms of underlying
	OptionModel             OptionModel // Model used to calculate theoretical values
	CurrentTime             *time.Time  // The algo.Timestamp from parent algo (should be updated from base layer)
}

func NewOptionTheo(strike float64, underlying *float64, expiry int, optionType string, denom bool, currentTime *time.Time) OptionTheo {
	optionTheo := OptionTheo{
		Strike:                  strike,
		UnderlyingPrice:         underlying,
		InterestRate:            0,
		Volatility:              -1,
		Expiry:                  expiry,
		TimeLeft:                0,
		OptionType:              optionType,
		Theo:                    -1,
		Delta:                   -1,
		Theta:                   -1,
		Gamma:                   -1,
		Vega:                    -1,
		WeightedVega:            -1,
		DenominatedInUnderlying: denom,
		OptionModel:             BlackScholes,
		CurrentTime:             currentTime,
	}
	optionTheo.GetTimeLeft()
	return optionTheo
}

func (o *OptionTheo) String() string {
	expiryTime := time.Unix(int64(o.Expiry/1000), 0)
	year := strconv.Itoa(expiryTime.Year())[2:4]
	month := strings.ToUpper(expiryTime.Month().String())[:3]
	day := strconv.Itoa(expiryTime.Day())
	return strconv.Itoa(int(o.Strike)) + "-" + day + month + year + "-" + strings.ToUpper(o.OptionType)
}

func (o *OptionTheo) getExpiryString() string {
	return time.Unix(int64(o.Expiry/1000), 0).UTC().String()
}

func (o *OptionTheo) GetTimeLeft() float64 {
	currentTimestamp := int((*o.CurrentTime).UnixNano() / 1000000)
	o.TimeLeft = float64(o.Expiry-currentTimestamp) / float64(1000*Day*365)
	return o.TimeLeft
}

// Black-scholes parameter
func (o *OptionTheo) calcD1(volatility float64) float64 {
	// // logger.Debugf("Calc D1 with underlying %v, strike %v, timeleft %v, interest %v\n", *o.UnderlyingPrice, o.String(), o.TimeLeft, o.InterestRate)
	return (math.Log(*o.UnderlyingPrice/o.Strike) + (o.InterestRate+(math.Pow(volatility, 2))/2)*o.GetTimeLeft()) / (volatility * math.Sqrt(o.GetTimeLeft()))
}

// Black-scholes parameter
func (o *OptionTheo) calcD2(volatility float64) float64 {
	return o.calcD1(volatility) - (volatility * math.Sqrt(o.GetTimeLeft()))
}

func (o *OptionTheo) calcPhi(d1 float64) float64 {
	return math.Exp(-math.Pow(d1, 2)/2) / math.Sqrt(2*PI)
}

func (o *OptionTheo) CalcTheo(calcGreeks bool) {
	if o.OptionModel == BlackScholes {
		o.CalcBlackScholesTheo(calcGreeks)
	} else if o.OptionModel == BinomialTree {
		// TODO set defaults for prob and numTimesteps
		o.CalcBinomialTreeTheo(.5, 100)
		if calcGreeks {
			o.CalcGreeks()
		}
	} else {
		fmt.Printf("Invalid OptionModel: %v\n", o.OptionModel)
	}
}

// Use Black-Scholes pricing model to calculate theoretical option value, or back out volatility from given theoretical option value.
// Calculate greeks if specified.
func (o *OptionTheo) CalcBlackScholesTheo(calcGreeks bool) {
	if (o.Volatility < 0 || math.IsNaN(o.Volatility)) && (o.Theo < 0 || math.IsNaN(o.Theo)) {
		o.Volatility = DefaultVolatility
		// logger.Debugf("Set volatility for %v to default volatility %v\n", o.String(), o.Volatility)
	}
	if o.Volatility < 0 || math.IsNaN(o.Volatility) {
		o.CalcVol(o.Theo)
	} else {
		norm := gaussian.NewGaussian(0, 1)
		d1 := o.calcD1(o.Volatility)
		d2 := o.calcD2(o.Volatility)
		if o.OptionType == "call" {
			if o.DenominatedInUnderlying {
				o.Theo = (*o.UnderlyingPrice*norm.Cdf(d1) - o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(d2)) / *o.UnderlyingPrice
			} else {
				o.Theo = (*o.UnderlyingPrice*norm.Cdf(d1) - o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(d2))
			}
		} else if o.OptionType == "put" {
			if o.DenominatedInUnderlying {
				o.Theo = (o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(-d2) - *o.UnderlyingPrice*norm.Cdf(-d1)) / *o.UnderlyingPrice
			} else {
				o.Theo = (o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(-d2) - *o.UnderlyingPrice*norm.Cdf(-d1))
			}
		}
		// // logger.Debugf("[%v] Calculated theo %v with vol %v, time %v, d1 %v, d2 %v\n", o.String(), o.Theo, o.Volatility, o.GetTimeLeft(), d1, d2)
	}
	if calcGreeks {
		o.CalcGreeks()
	}
}

// Calculate greeks (delta, gamma, theta) for a given option (option volatility must be known)
func (o *OptionTheo) CalcGreeks() {
	// // logger.Debugf("Calculating greeks for %v with vol %v\n", o.String(), o.Volatility)
	if o.Volatility < 0 {
		// logger.Debugf("Volatility must be known for %v to calculate greeks.\n", o.String())
		return
	}
	dist := gaussian.NewGaussian(0, 1)
	d1 := o.calcD1(o.Volatility)
	d2 := o.calcD2(o.Volatility)
	phi := o.calcPhi(d1)
	o.CalcDelta(d1, dist)
	o.CalcGamma(phi)
	o.CalcTheta(phi, d2, dist)
	o.CalcVega(phi)
	o.CalcWeightedVega(phi)
	// // logger.Debugf("[%v] Theo %v, Delta %v, Gamma %v, Theta %v, Vega %v\n", o.String(), o.Theo, o.Delta, o.Gamma, o.Theta, o.Vega)
}

// Return the black-scholes theoretical value for an option for a given volatility value, but do not store it.
func (o *OptionTheo) GetBlackScholesTheo(volatility float64) float64 {
	norm := gaussian.NewGaussian(0, 1)
	d1 := o.calcD1(volatility)
	d2 := o.calcD2(volatility)
	theo := 0.
	if o.OptionType == "call" {
		theo = *o.UnderlyingPrice*norm.Cdf(d1) - o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(d2)
	} else if o.OptionType == "put" {
		theo = o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(-d2) - *o.UnderlyingPrice*norm.Cdf(-d1)
	}
	// // logger.Debugf("got theo %v with vol %v, d1 %v d2 %v\n", theo, volatility, d1, d2)
	if o.DenominatedInUnderlying {
		return theo / *o.UnderlyingPrice
	}
	return theo
}

// Use newton raphson method to find volatility given an option price
func (o *OptionTheo) CalcVol(price float64) {
	// logger.Debugf("Calculating vol for %v with theo %v, time left %v, underlying %v", o.String(), o.Theo, o.GetTimeLeft(), *o.UnderlyingPrice)
	if price > 0 {
		norm := gaussian.NewGaussian(0, 1)
		v := math.Sqrt(2*PI/o.GetTimeLeft()) * price
		// // logger.Debugf("initial vol: %v\n", v)
		for i := 0; i < 10000; i++ {
			d1 := o.calcD1(v)
			d2 := o.calcD2(v)
			vega := *o.UnderlyingPrice * norm.Pdf(d1) * math.Sqrt(o.GetTimeLeft())
			// // logger.Debugf("Underlying %v, pdf %v, time el %v\n", *o.UnderlyingPrice, norm.Pdf(d1), math.Sqrt(o.GetTimeLeft()))
			cp := 1.0
			if o.OptionType == "put" {
				cp = -1.0
			}
			var theo0 float64
			if o.DenominatedInUnderlying {
				theo0 = (cp*(*o.UnderlyingPrice)*norm.Cdf(cp*d1) - cp*o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(cp*d2)) / *o.UnderlyingPrice
			} else {
				theo0 = (cp*(*o.UnderlyingPrice)*norm.Cdf(cp*d1) - cp*o.Strike*math.Exp(-o.InterestRate*o.GetTimeLeft())*norm.Cdf(cp*d2))
			}
			v = v - (theo0-price)/vega
			// // logger.Debugf("Next vol: %v with theo %v, d1 %v d2 %v vega %v\n", v, theo0, d1, d2, vega)
			if math.Abs(theo0-price) < math.Pow(10, -25) {
				// // logger.Debugf("D1: %v, d2: %v\n", d1, d2)
				break
			}
		}
		// logger.Debugf("Calculated vol %v for %v, price %v\n", v, o.String(), price)
		o.Volatility = v
	} else {
		// logger.Debugf("Can only calc vol with positive price. Found %v\n", price)
	}
}

func (o *OptionTheo) CalcDelta(d1 float64, dist *gaussian.Gaussian) {
	if o.OptionType == "call" {
		o.Delta = dist.Cdf(d1)
	} else if o.OptionType == "put" {
		o.Delta = dist.Cdf(d1) - 1
	}
}

func (o *OptionTheo) CalcGamma(phi float64) {
	o.Gamma = (phi / (*o.UnderlyingPrice * o.Volatility * math.Sqrt(o.GetTimeLeft())))
}

func (o *OptionTheo) CalcTheta(phi float64, d2 float64, dist *gaussian.Gaussian) {
	o.Theta = (((-(*o.UnderlyingPrice) * phi * o.Volatility) / (2 * math.Sqrt(o.GetTimeLeft()))) -
		(o.InterestRate * o.Strike * math.Exp(-o.InterestRate*o.GetTimeLeft()) * dist.Cdf(d2))) / 365
}

func (o *OptionTheo) CalcVega(phi float64) {
	o.Vega = *o.UnderlyingPrice * phi * math.Sqrt(o.GetTimeLeft())
}

// Calculate weighted vega for an option by calculating vega as a fraction of at-the-money vega
func (o *OptionTheo) CalcWeightedVega(phi float64) {
	atmOption := NewOptionTheo(
		o.Strike,
		&o.Strike,
		o.Expiry,
		o.OptionType,
		o.DenominatedInUnderlying,
		o.CurrentTime,
	)
	// TODO adjust volatility for ATM option?
	atmOption.Volatility = o.Volatility
	atmOption.CalcTheo(false)
	atmPhi := atmOption.calcPhi(atmOption.calcD1(atmOption.Volatility))
	atmOption.CalcVega(atmPhi)
	o.CalcTheo(false)
	o.CalcVega(phi)
	o.WeightedVega = o.Vega / atmOption.Vega
}

// Get an option's PNL at expiration
func (o *OptionTheo) GetExpiryValue(currentPrice float64) float64 {
	expiryValue := 0.
	if o.OptionType == "call" {
		expiryValue = (currentPrice - o.Strike) / currentPrice
	} else if o.OptionType == "put" {
		expiryValue = (o.Strike - currentPrice) / currentPrice
	}
	if expiryValue < 0 {
		expiryValue = 0
	}
	return expiryValue
}

// Recursively calculate the expected values of underlying price.
// TODO: can be made more efficient by assuming paths can intersect (i.e. up -> down yields same node as down -> up)
// Can be done with binomial tree indexing instead of indexing by path string:
//			4
//		2
//	1		5
//		3
//			6
//
// 	0	1	2
//   timestep

// Stopping conditions for binomial walk
const maxPrice = 20000
const minPrice = 2000
const minProb = .00001

func (o *OptionTheo) binomialWalk(move float64, prob float64, currentPrice float64, currentProb float64, path string,
	evSum *float64, timestepsLeft int, walkCache map[string]*float64) {
	value, ok := walkCache[path]
	if ok {
		// // logger.Debugf("Loaded EV %v for path %v\n", *value, path)
		*evSum += *value
		return
	} else if timestepsLeft <= 0 || currentPrice > maxPrice || currentPrice < minPrice || currentProb < minProb {
		ev := 0.
		if o.OptionType == "call" {
			ev = (currentPrice - o.Strike) * currentProb
		} else if o.OptionType == "put" {
			ev = (o.Strike - currentPrice) * currentProb
		}
		if ev < 0 {
			ev = 0
		}
		*evSum += ev
		walkCache[path] = &ev
		// log.Printf("Cached EV %v for path %v\n", ev, path)
		// // logger.Debugf("Cached EV %v for path %v\n", ev, path)
		return
	}
	currentPrice = currentPrice * (1 + move)
	currentProb = currentProb * prob
	if move < 0 {
		move *= -1
		prob = 1 - prob
		path += "d"
	} else {
		path += "u"
	}
	// Walk up
	o.binomialWalk(move, prob, currentPrice, currentProb, path, evSum, timestepsLeft-1, walkCache)
	// Walk down
	o.binomialWalk(-move, 1-prob, currentPrice, currentProb, path, evSum, timestepsLeft-1, walkCache)
}

// Calculate the theoretical value of an option based on a binary tree model
// We can calculate the appropriate move for each timestep based on volatility of underlying and time to expiry
// Param prob: probability of an upmove at each timestep
// Param numTimesteps: number of timesteps for the binomial tree traversal
func (o *OptionTheo) CalcBinomialTreeTheo(prob float64, numTimesteps int) {
	timestep := o.GetTimeLeft() / float64(numTimesteps)
	move := o.Volatility * math.Sqrt(timestep)
	// // logger.Debugf("Calculating binomial tree theo with numTimesteps %v, move %v, prob %v, volatility %v\n", numTimesteps, move, prob, o.volatility)
	path := ""
	walkCache := make(map[string]*float64) // Stores an ev for a path whose ev is known
	evSum := 0.
	o.binomialWalk(move, prob, *o.UnderlyingPrice, 1, path, &evSum, numTimesteps, walkCache)
	// Calculate binomial tree theo quoted in terms of underlying price
	// // logger.Debugf("EV sum: %v, binomialTheo: %v, move: %v\n", evSum, o.binomialTheo, move)
	if o.DenominatedInUnderlying {
		o.Theo = evSum / *o.UnderlyingPrice
	} else {
		o.Theo = evSum
	}
}
