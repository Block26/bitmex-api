package options

import (
	"math"

	"github.com/chobie/go-gaussian"
)

const PI float64 = 3.14159265359
const day = 86400

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

func (self *OptionTheo) calcD1(volatility float64) float64 {
	return (math.Log(self.uPrice/self.strike) + (self.r+math.Pow(self.volatility, 2)/2)*self.timeLeft) / (volatility * math.Sqrt(self.timeLeft))
}

func (self *OptionTheo) calcD2(volatility float64) float64 {
	return (math.Log(self.uPrice/self.strike) + (self.r-math.Pow(self.volatility, 2)/2)*self.timeLeft) / (volatility * math.Sqrt(self.timeLeft))
}

// Use Black-Scholes pricing model to calculate theoretical option value
func (self *OptionTheo) calcBlackScholesTheo(calcGreeks bool) {
	norm := gaussian.NewGaussian(0, 1)
	td1 := self.calcD1(self.volatility)
	td2 := self.calcD2(self.volatility)
	nPrime := math.Pow((2*PI), -(1/2)) * math.Exp(math.Pow(-0.5*(td1), 2))
	if self.theo < 0 {
		if self.optionType == "call" {
			self.theo = self.uPrice*norm.Cdf(td1) - self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(td2)
		} else if self.optionType == "put" {
			self.theo = self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(-td2) - self.uPrice*norm.Cdf(-td1)
		}
	} else if self.volatility < 0 {
		self.volatility = self.impliedVol()
	}
	if calcGreeks {
		if self.optionType == "call" {
			self.theo = self.uPrice*norm.Cdf(td1) - self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(td2)
			self.delta = norm.Cdf(td1)
			self.gamma = (nPrime / (self.uPrice * self.volatility * math.Pow(self.timeLeft, (1/2))))
			self.theta = (nPrime)*(-self.uPrice*self.volatility*0.5/math.Sqrt(self.timeLeft)) - self.r*self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(td2)
		} else if self.optionType == "put" {
			self.theo = self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(-td2) - self.uPrice*norm.Cdf(-td1)
			self.delta = norm.Cdf(td1) - 1
			self.gamma = (nPrime / (self.uPrice * self.volatility * math.Pow(self.timeLeft, (1/2))))
			self.theta = (nPrime)*(-self.uPrice*self.volatility*0.5/math.Sqrt(self.timeLeft)) + self.r*self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(-td2)
		}
	}

	// Convert theo to be quoted in terms of underlying
	self.theo = self.theo / self.uPrice
}

// Use newton raphson method to find volatility
func (self *OptionTheo) impliedVol() float64 {
	norm := gaussian.NewGaussian(0, 1)
	v := math.Sqrt(2*PI/self.timeLeft) * self.theo / self.uPrice
	for i := 0; i < 100; i++ {
		d1 := self.calcD1(v)
		d2 := self.calcD2(v)
		vega := self.uPrice * norm.Pdf(d1) * math.Sqrt(self.timeLeft)
		cp := 1.0
		if self.optionType == "put" {
			cp = -1.0
		}
		theo0 := cp*self.uPrice*norm.Cdf(cp*d1) - cp*self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(cp*d2)
		v = v - (theo0-self.theo)/vega
		if math.Abs(theo0-self.theo) < math.Pow(10, -25) {
			break
		}
	}
	return v
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
func (self *OptionTheo) binomialWalk(move float64, prob float64, currentPrice float64, currentProb float64, path string,
	evSum *float64, timestepsLeft int, walkCache map[string]*float64) {
	value, ok := walkCache[path]
	if ok {
		// fmt.Printf("Loaded EV %v for path %v\n", *value, path)
		*evSum += *value
		return
	} else if timestepsLeft <= 0 || currentPrice > maxPrice || currentPrice < minPrice || currentProb < minProb {
		ev := 0.
		if self.optionType == "call" {
			ev = (currentPrice - self.strike) * currentProb
		} else if self.optionType == "put" {
			ev = (self.strike - currentPrice) * currentProb
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
	self.binomialWalk(move, prob, currentPrice, currentProb, path, evSum, timestepsLeft-1, walkCache)
	// Walk down
	self.binomialWalk(-move, 1-prob, currentPrice, currentProb, path, evSum, timestepsLeft-1, walkCache)
}

// Calculate the theoretical value of an option based on a binary tree model
// We can calculate the appropriate move for each timestep based on volatility of underlying and time to expiry
// Param prob: probability of an upmove at each timestep
// Param numTimesteps: number of timesteps for the binomial tree traversal
func (self *OptionTheo) calcBinomialTreeTheo(prob float64, numTimesteps int) {
	timestep := self.timeLeft / float64(numTimesteps)
	move := self.volatility * math.Sqrt(timestep)
	// fmt.Printf("Calculating binomial tree theo with numTimesteps %v, move %v, prob %v, volatility %v\n", numTimesteps, move, prob, self.volatility)
	path := ""
	walkCache := make(map[string]*float64) // Stores an ev for a path whose ev is known
	evSum := 0.
	self.binomialWalk(move, prob, self.uPrice, 1, path, &evSum, numTimesteps, walkCache)
	// Calculate binomial tree theo quoted in terms of underlying price
	self.binomialTheo = evSum / self.uPrice
	// fmt.Printf("EV sum: %v, binomialTheo: %v, move: %v\n", evSum, self.binomialTheo, move)
}
