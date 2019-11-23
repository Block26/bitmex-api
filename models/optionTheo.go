package models

import (
	"math"

	"github.com/chobie/go-gaussian"
)

type OptionTheo struct {
	strike       float64 // Strike price
	uPrice       float64 // Underlying price
	r            float64 // Risk free rate (assume 0)
	volatility   float64 // Implied volatility
	currentTime  int     // Current time (ms)
	expiry       int     // Expiration date (ms)
	timeLeft     float64 // Time left until expiry (days)
	optionType   string  // "call" or "put"
	theo         float64 // Theoretical value calculated via Black-Scholes
	binomialTheo float64 // Theoretical value calculated via binomial tree
	delta        float64 // Change in theo wrt. 1 USD change in uPrice
	theta        float64 // Change in theo wrt. 1 day decrease in timeLeft
	gamma        float64 // Change in delta wrt. 1 USD change in uPrice
	vega         float64 // Change in theo wrt. 1% increase in volatility
}

const PI float64 = 3.14159265359
const day = 86400

// Either theo or volatility is unknown (pass in -1.0 for unknown values)
func NewOptionTheo(optionType string, uPrice float64, strike float64,
	currentTime int, expiry int, r float64,
	volatility float64, theo float64) *OptionTheo {
	o := &OptionTheo{
		strike:      strike,
		uPrice:      uPrice,
		r:           r,
		currentTime: currentTime,
		expiry:      expiry,
		timeLeft:    GetTimeLeft(currentTime, expiry),
		optionType:  optionType,
		volatility:  volatility,
		theo:        theo,
	}
	return o
}

// Times in ms; return time in days
func GetTimeLeft(currentTime int, expiry int) float64 {
	return float64(expiry-currentTime) / float64(1000*day*365)
}

func (o *OptionTheo) calcD1(volatility float64) float64 {
	return (math.Log(o.uPrice/o.strike) + (o.r+math.Pow(o.volatility, 2)/2)*o.timeLeft) / (volatility * math.Sqrt(o.timeLeft))
}

func (o *OptionTheo) calcD2(volatility float64) float64 {
	return (math.Log(o.uPrice/o.strike) + (o.r-math.Pow(o.volatility, 2)/2)*o.timeLeft) / (volatility * math.Sqrt(o.timeLeft))
}

// Use Black-Scholes pricing model to calculate theoretical option value
func (o *OptionTheo) calcBlackScholesTheo(calcGreeks bool) {
	norm := gaussian.NewGaussian(0, 1)
	td1 := o.calcD1(o.volatility)
	td2 := o.calcD2(o.volatility)
	nPrime := math.Pow((2*PI), -(1/2)) * math.Exp(math.Pow(-0.5*(td1), 2))
	if o.theo < 0 {
		if o.optionType == "call" {
			o.theo = o.uPrice*norm.Cdf(td1) - o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(td2)
		} else if o.optionType == "put" {
			o.theo = o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(-td2) - o.uPrice*norm.Cdf(-td1)
		}
	} else if o.volatility < 0 {
		o.volatility = o.impliedVol()
	}
	if calcGreeks {
		if o.optionType == "call" {
			o.theo = o.uPrice*norm.Cdf(td1) - o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(td2)
			o.delta = norm.Cdf(td1)
			o.gamma = (nPrime / (o.uPrice * o.volatility * math.Pow(o.timeLeft, (1/2))))
			o.theta = (nPrime)*(-o.uPrice*o.volatility*0.5/math.Sqrt(o.timeLeft)) - o.r*o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(td2)
		} else if o.optionType == "put" {
			o.theo = o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(-td2) - o.uPrice*norm.Cdf(-td1)
			o.delta = norm.Cdf(td1) - 1
			o.gamma = (nPrime / (o.uPrice * o.volatility * math.Pow(o.timeLeft, (1/2))))
			o.theta = (nPrime)*(-o.uPrice*o.volatility*0.5/math.Sqrt(o.timeLeft)) + o.r*o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(-td2)
		}
	}
	// Convert theo to be quoted in terms of underlying
	o.theo = o.theo / o.uPrice
}

// Use newton raphson method to find volatility
func (o *OptionTheo) impliedVol() float64 {
	norm := gaussian.NewGaussian(0, 1)
	v := math.Sqrt(2*PI/o.timeLeft) * o.theo / o.uPrice
	for i := 0; i < 100; i++ {
		d1 := o.calcD1(v)
		d2 := o.calcD2(v)
		vega := o.uPrice * norm.Pdf(d1) * math.Sqrt(o.timeLeft)
		cp := 1.0
		if o.optionType == "put" {
			cp = -1.0
		}
		theo0 := cp*o.uPrice*norm.Cdf(cp*d1) - cp*o.strike*math.Exp(-o.r*o.timeLeft)*norm.Cdf(cp*d2)
		v = v - (theo0-o.theo)/vega
		if math.Abs(theo0-o.theo) < math.Pow(10, -25) {
			break
		}
	}
	return v
}

// Get an option's PNL at expiration
func (o *OptionTheo) getExpiryValue(currentPrice float64) float64 {
	expiryValue := 0.
	if o.optionType == "call" {
		expiryValue = currentPrice - o.strike
	} else if o.optionType == "put" {
		expiryValue = o.strike - currentPrice
	}
	if expiryValue < 0 {
		expiryValue = 0
	}
	return expiryValue
}

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

// Recursively calculate the expected values of underlying price
func (o *OptionTheo) binomialWalk(move float64, prob float64, currentPrice float64, currentProb float64, path string,
	evSum *float64, timestepsLeft int, walkCache map[string]*float64) {
	value, ok := walkCache[path]
	if ok {
		// fmt.Printf("Loaded EV %v for path %v\n", *value, path)
		*evSum += *value
		return
	} else if timestepsLeft <= 0 || currentPrice > maxPrice || currentPrice < minPrice || currentProb < minProb {
		ev := 0.
		if o.optionType == "call" {
			ev = (currentPrice - o.strike) * currentProb
		} else if o.optionType == "put" {
			ev = (o.strike - currentPrice) * currentProb
		}
		if ev < 0 {
			ev = 0
		}
		*evSum += ev
		walkCache[path] = &ev
		// log.Printf("Cached EV %v for path %v\n", ev, path)
		// fmt.Printf("Cached EV %v for path %v\n", ev, path)
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
func (o *OptionTheo) calcBinomialTreeTheo(prob float64, numTimesteps int) {
	timestep := o.timeLeft / float64(numTimesteps)
	move := o.volatility * math.Sqrt(timestep)
	// fmt.Printf("Calculating binomial tree theo with numTimesteps %v, move %v, prob %v, volatility %v\n", numTimesteps, move, prob, o.volatility)
	path := ""
	walkCache := make(map[string]*float64) // Stores an ev for a path whose ev is known
	evSum := 0.
	o.binomialWalk(move, prob, o.uPrice, 1, path, &evSum, numTimesteps, walkCache)
	// Calculate binomial tree theo quoted in terms of underlying price
	o.binomialTheo = evSum / o.uPrice
	// fmt.Printf("EV sum: %v, binomialTheo: %v, move: %v\n", evSum, o.binomialTheo, move)
}
