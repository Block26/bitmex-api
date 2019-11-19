package options

import (
	"fmt"
	"math"

	"github.com/chobie/go-gaussian"
)

const PI float64 = 3.14159265359
const day = 86400

type OptionData struct {
	strike      float64 // strike price
	uPrice      float64 // underlying price
	r           float64 // risk free rate
	volatility  float64 // implied volatility
	currentTime int     // current time
	expiry      int     // expiration date
	timeLeft    float64 // distance between exp and current
	optionType  string  // "call" or "put"
	theo        float64 // derived from info above
	delta       float64 // derived from info above
	theta       float64 // derived from info above
	gamma       float64 // derived from info above
}

// Either theo or volatility is unknown (pass in -1.0 for unknown values)
func NewOptionData(optionType string, uPrice float64, strike float64,
	currentTime int, expiry int, r float64,
	volatility float64, theo float64) *OptionData {
	o := &OptionData{
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
	// fmt.Printf("Parsed new option data")
	// fmt.Printf("strike: %v", o.strike)
	// fmt.Printf("uPrice: %v", o.uPrice)
	// fmt.Printf("r: %v", o.r)
	// fmt.Printf("currentTime: %v", o.currentTime)
	// fmt.Printf("expiry: %v", o.expiry)
	// fmt.Printf("timeLeft: %v", o.timeLeft)
	// fmt.Printf("optionType: %v", o.optionType)
	// fmt.Printf("volatility: %v", o.volatility)
	// fmt.Printf("theo: %v", o.theo)
	return o
}

//Times in ms
func GetTimeLeft(currentTime int, expiry int) float64 {
	return float64(expiry-currentTime) / float64(1000*day*365)
}

func (self *OptionData) d1() float64 {
	return (math.Log(self.uPrice/self.strike) + (self.r+math.Pow(self.volatility, 2)/2)*self.timeLeft) / (self.volatility * math.Sqrt(self.timeLeft))
}

func (self *OptionData) d2() float64 {
	return (math.Log(self.uPrice/self.strike) + (self.r-math.Pow(self.volatility, 2)/2)*self.timeLeft) / (self.volatility * math.Sqrt(self.timeLeft))
}

// calculate Black Scholes theo and greeks
func (self *OptionData) getBlackScholes(calcGreeks bool) float64 {
	norm := gaussian.NewGaussian(0, 1)
	td1 := self.d1()
	td2 := self.d2()
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

	// if self.theo < 0 {
	// 	d1 := (math.Log(self.uPrice/self.strike) + (self.r + (math.Pow(self.volatility, 2.0)/2.0)*self.timeLeft)) / (self.volatility * math.Sqrt(self.timeLeft))
	// 	d2 := d1 - (self.volatility * math.Sqrt(self.timeLeft))
	// 	presentValue := self.strike * math.Exp(-self.r*self.timeLeft)
	// 	if self.optionType == "call" {
	// 		self.theo = (self.uPrice * norm.Cdf(d1)) - (presentValue * norm.Cdf(d2))
	// 	} else if self.optionType == "put" {
	// 		self.theo = (norm.Cdf(-d2) * presentValue) - (norm.Cdf(-d1) * self.uPrice)
	// 	}
	// 	if self.theo < 0 {
	// 		self.theo = 0
	// 	}
	// } else if self.volatility < 0 {
	// 	self.volatility = self.impliedVol()
	// }

	// Convert theo to be quoted in terms of underlying
	self.theo = self.theo / self.uPrice
	return self.theo
}

// use newton raphson method to find volatility
func (self *OptionData) impliedVol() float64 {
	norm := gaussian.NewGaussian(0, 1)
	v := math.Sqrt(2*PI/self.timeLeft) * self.theo / self.uPrice
	//fmt.Printf(“ - initial vol: %v\n”, v)
	for i := 0; i < 100; i++ {
		d1 := (math.Log(self.uPrice/self.strike) + (self.r+0.5*math.Pow(v, 2))*self.timeLeft) / (v * math.Sqrt(self.timeLeft))
		d2 := d1 - v*math.Sqrt(self.timeLeft)
		vega := self.uPrice * norm.Pdf(d1) * math.Sqrt(self.timeLeft)
		cp := 1.0
		if self.optionType == "put" {
			cp = -1.0
		}
		theo0 := cp*self.uPrice*norm.Cdf(cp*d1) - cp*self.strike*math.Exp(-self.r*self.timeLeft)*norm.Cdf(cp*d2)
		v = v - (theo0-self.theo)/vega
		//fmt.Printf(“ - next vol %v : %v / %v \n”, i, v,
		//             math.Pow(10, -25))
		if math.Abs(theo0-self.theo) < math.Pow(10, -25) {
			break
		}
	}
	return v
}

func GetOptionValue(optionType string, uPrice float64, strike float64, currentTime int, expiry int, method string, impliedVol float64) float64 {
	// Assume interest rate of 0
	r := 0.0
	value := -1.0
	optionData := NewOptionData(optionType, uPrice, strike, currentTime, expiry, r, impliedVol, value)
	if method == "blackScholes" {
		value = optionData.getBlackScholes(false)
	} else {
		fmt.Printf("Option valuation method %v not supported", method)
	}
	return value
}
