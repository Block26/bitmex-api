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
	gamma        float64 // Change in delta wrt. 1USD change in uPrice
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

// Recursively calculate the expected values of underlying price
func BinomialWalk(currentProb float64, move float64, prob float64, timestepsLeft int, probs []float64, currentPrice float64, prices []float64) {
	if timestepsLeft <= 0 {
		probs[len(probs)] = currentProb
		prices[len(prices)] = currentPrice
		return
	}
	currentPrice = currentPrice * (1 + move)
	currentProb = currentProb * prob
	// Walk in same direction
	BinomialWalk(currentProb, move, prob, timestepsLeft-1, probs, currentPrice, prices)
	// Walk in opposite direction
	BinomialWalk(currentProb, -move, 1-prob, timestepsLeft-1, probs, currentPrice, prices)
}

// Calculate theoretical option value based on percentage move, probability of up move, and length of timesteps (in seconds)
// Param move: magnitude of each move at each timestep, in terms of fraction (i.e. 1% -> 0.01)
// Param prob: probability of an up move (i.e. 0.5), downmove assumed with complementary probability
// Param timestep: number of seconds for each timestep in the binomial walk
func (self *OptionTheo) calcBinomialTreeTheo(move float64, prob float64, timestep float64) {
	numTimesteps := int(math.Ceil(float64(self.expiry-self.currentTime) / 1000))
	probs := make([]float64, numTimesteps)
	prices := make([]float64, numTimesteps)
	BinomialWalk(1, move, prob, numTimesteps, probs, self.uPrice, prices)
	evSum := 0.0
	if self.optionType == "call" {
		for i := 0; i < len(prices); i++ {
			ev := probs[i] * (prices[i] - self.strike)
			if ev < 0 {
				ev = 0
			}
			evSum += ev
		}
	} else if self.optionType == "put" {
		for i := 0; i < len(prices); i++ {
			ev := probs[i] * (self.strike - prices[i])
			if ev < 0 {
				ev = 0
			}
			evSum += ev
		}
	}
	// Calculate binomial tree theo quoted in terms of underlying price
	self.binomialTheo = evSum / float64(len(prices)) / self.uPrice
}
