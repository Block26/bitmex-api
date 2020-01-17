package models

import "math"

// SearchParameter specify the min and max range as well as the decimal to round to.
type SearchParameter struct {
	min      float64
	max      float64
	decimals int
	value    float64
}

// NewSearchParameter Create a new search domain for a parameter to search between a min and max, and round to a decimal place
func NewSearchParameter(min float64, max float64, decimals int) SearchParameter {
	return SearchParameter{
		min:      min,
		max:      max,
		decimals: decimals,
	}
}

func (p SearchParameter) GetMin() float64 {
	return p.min
}

func (p SearchParameter) GetMax() float64 {
	return p.max
}

func (p SearchParameter) GetFloatValue() float64 {
	return p.value
}

func (p SearchParameter) GetIntValue() int {
	return int(p.value)
}

func (p SearchParameter) GetBoolValue() bool {
	return p.value >= 0.5
}

func (p SearchParameter) SetValue(value float64) SearchParameter {
	p.value = ToFixed(math.Max(p.min, math.Min(value, p.max)), p.decimals)
	return p
}

func ToFixed(num float64, precision int) float64 {
	output := math.Pow(10, float64(precision))
	return float64(round(num*output)) / output
}

func round(num float64) int {
	return int(num + math.Copysign(0.5, num))
}
