package models

import (
	"fmt"
	"testing"
	"time"
)

func CheckGreeks(t *testing.T, optionTheo OptionTheo, theo, delta, gamma, theta, vega, weightedVega float64) {
	if optionTheo.Theo != theo {
		t.Errorf("Bad Theo: %v, expected %v\n", optionTheo.Theo, theo)
	}
	if optionTheo.Delta != delta {
		t.Errorf("Bad Delta: %v, expected %v\n", optionTheo.Delta, delta)
	}
	if optionTheo.Gamma != gamma {
		t.Errorf("Bad Gamma: %v, expected %v\n", optionTheo.Gamma, gamma)
	}
	if optionTheo.Theta != theta {
		t.Errorf("Bad Theta: %v, expected %v\n", optionTheo.Theta, theta)
	}
	if optionTheo.Vega != vega {
		t.Errorf("Bad Vega: %v, expected %v\n", optionTheo.Vega, vega)
	}
	if optionTheo.WeightedVega != weightedVega {
		t.Errorf("Bad Vega: %v, expected %v\n", optionTheo.WeightedVega, weightedVega)
	}
}

func TestOTMCall(t *testing.T) {

	currentTime := time.Now()
	currentTimestamp := int(currentTime.UnixNano() / 1000000)
	expiry := currentTimestamp + (Day * 7 * 1000)
	underlyingPrice := 9000.
	optionTheo := NewOptionTheo(
		10000.,
		&underlyingPrice,
		expiry,
		"call",
		true,
		&currentTime,
	)
	optionTheo.Volatility = .75
	fmt.Printf("Running theo calculations on %v...\n", optionTheo.String())
	optionTheo.CalcTheo(true)
	fmt.Printf("Got values:\n  Theo: %v (%v USD)\n  Delta: %v\n  Gamma: %v\n  Theta: %v\n  Vega: %v\n  Weighted Vega: %v\n",
		optionTheo.Theo, optionTheo.Theo*underlyingPrice, optionTheo.Delta, optionTheo.Gamma, optionTheo.Theta, optionTheo.Vega, optionTheo.WeightedVega)
	expectedTheo := 0.008866327080024954
	expectedDelta := 0.1679044431256924
	expectedGamma := 0.0002685627517503254
	expectedTheta := -16.762178598115344
	expectedVega := 312.89400049815305
	expectedWeightedVega := 0.5671137937331275
	CheckGreeks(t, optionTheo, expectedTheo, expectedDelta, expectedGamma, expectedTheta, expectedVega, expectedWeightedVega)
}

func TestITMCall(t *testing.T) {

	currentTime := time.Now()
	currentTimestamp := int(currentTime.UnixNano() / 1000000)
	expiry := currentTimestamp + (Day * 7 * 1000)
	underlyingPrice := 11000.
	optionTheo := NewOptionTheo(
		10000.,
		&underlyingPrice,
		expiry,
		"call",
		true,
		&currentTime,
	)
	optionTheo.Volatility = .75
	fmt.Printf("Running theo calculations on %v...\n", optionTheo.String())
	optionTheo.CalcTheo(true)
	fmt.Printf("Got values:\n  Theo: %v (%v USD)\n  Delta: %v\n  Gamma: %v\n  Theta: %v\n  Vega: %v\n  Weighted Vega: %v\n",
		optionTheo.Theo, optionTheo.Theo*underlyingPrice, optionTheo.Delta, optionTheo.Gamma, optionTheo.Theta, optionTheo.Vega, optionTheo.WeightedVega)
	expectedTheo := 0.10052921761589377
	expectedDelta := 0.8338716543127831
	expectedGamma := 0.0002182314071722745
	expectedTheta := -20.34708924748347
	expectedVega := 379.81233261969146
	expectedWeightedVega := 0.6884018629812404
	CheckGreeks(t, optionTheo, expectedTheo, expectedDelta, expectedGamma, expectedTheta, expectedVega, expectedWeightedVega)
}

func TestOTMPut(t *testing.T) {

	currentTime := time.Now()
	currentTimestamp := int(currentTime.UnixNano() / 1000000)
	expiry := currentTimestamp + (Day * 7 * 1000)
	underlyingPrice := 11000.
	optionTheo := NewOptionTheo(
		10000.,
		&underlyingPrice,
		expiry,
		"put",
		true,
		&currentTime,
	)
	optionTheo.Volatility = .75
	fmt.Printf("Running theo calculations on %v...\n", optionTheo.String())
	optionTheo.CalcTheo(true)
	fmt.Printf("Got values:\n  Theo: %v (%v USD)\n  Delta: %v\n  Gamma: %v\n  Theta: %v\n  Vega: %v\n  Weighted Vega: %v\n",
		optionTheo.Theo, optionTheo.Theo*underlyingPrice, optionTheo.Delta, optionTheo.Gamma, optionTheo.Theta, optionTheo.Vega, optionTheo.WeightedVega)
	expectedTheo := 0.009620126706802817
	expectedDelta := -0.1661283456872169
	expectedGamma := 0.0002182314071722745
	expectedTheta := -20.34708924748347
	expectedVega := 379.81233261969146
	expectedWeightedVega := 0.6884018629812404
	CheckGreeks(t, optionTheo, expectedTheo, expectedDelta, expectedGamma, expectedTheta, expectedVega, expectedWeightedVega)
}

func TestITMPut(t *testing.T) {

	currentTime := time.Now()
	currentTimestamp := int(currentTime.UnixNano() / 1000000)
	expiry := currentTimestamp + (Day * 7 * 1000)
	underlyingPrice := 9000.
	optionTheo := NewOptionTheo(
		10000.,
		&underlyingPrice,
		expiry,
		"put",
		true,
		&currentTime,
	)
	optionTheo.Volatility = .75
	fmt.Printf("Running theo calculations on %v...\n", optionTheo.String())
	optionTheo.CalcTheo(true)
	fmt.Printf("Got values:\n  Theo: %v (%v USD)\n  Delta: %v\n  Gamma: %v\n  Theta: %v\n  Vega: %v\n  Weighted Vega: %v\n",
		optionTheo.Theo, optionTheo.Theo*underlyingPrice, optionTheo.Delta, optionTheo.Gamma, optionTheo.Theta, optionTheo.Vega, optionTheo.WeightedVega)
	expectedTheo := 0.11997743819113607
	expectedDelta := -0.8320955568743076
	expectedGamma := 0.0002685627517503254
	expectedTheta := -16.762178598115344
	expectedVega := 312.89400049815305
	expectedWeightedVega := 0.5671137937331275
	CheckGreeks(t, optionTheo, expectedTheo, expectedDelta, expectedGamma, expectedTheta, expectedVega, expectedWeightedVega)
}

func TestATMCall(t *testing.T) {

	currentTime := time.Now()
	currentTimestamp := int(currentTime.UnixNano() / 1000000)
	expiry := currentTimestamp + (Day * 7 * 1000)
	underlyingPrice := 10000.
	optionTheo := NewOptionTheo(
		10000.,
		&underlyingPrice,
		expiry,
		"call",
		true,
		&currentTime,
	)
	optionTheo.Volatility = .75
	fmt.Printf("Running theo calculations on %v...\n", optionTheo.String())
	optionTheo.CalcTheo(true)
	fmt.Printf("Got values:\n  Theo: %v (%v USD)\n  Delta: %v\n  Gamma: %v\n  Theta: %v\n  Vega: %v\n  Weighted Vega: %v\n",
		optionTheo.Theo, optionTheo.Theo*underlyingPrice, optionTheo.Delta, optionTheo.Gamma, optionTheo.Theta, optionTheo.Vega, optionTheo.WeightedVega)
	expectedTheo := 0.04141709263183366
	expectedDelta := 0.5207085463159168
	expectedGamma := 0.0003835840907444611
	expectedTheta := -29.556993293665666
	expectedVega := 551.730541481759
	expectedWeightedVega := 1.
	CheckGreeks(t, optionTheo, expectedTheo, expectedDelta, expectedGamma, expectedTheta, expectedVega, expectedWeightedVega)
}

func TestATMPut(t *testing.T) {

	currentTime := time.Now()
	currentTimestamp := int(currentTime.UnixNano() / 1000000)
	expiry := currentTimestamp + (Day * 7 * 1000)
	underlyingPrice := 10000.
	optionTheo := NewOptionTheo(
		10000.,
		&underlyingPrice,
		expiry,
		"put",
		true,
		&currentTime,
	)
	optionTheo.Volatility = .75
	fmt.Printf("Running theo calculations on %v...\n", optionTheo.String())
	optionTheo.CalcTheo(true)
	fmt.Printf("Got values:\n  Theo: %v (%v USD)\n  Delta: %v\n  Gamma: %v\n  Theta: %v\n  Vega: %v\n  Weighted Vega: %v\n",
		optionTheo.Theo, optionTheo.Theo*underlyingPrice, optionTheo.Delta, optionTheo.Gamma, optionTheo.Theta, optionTheo.Vega, optionTheo.WeightedVega)
	expectedTheo := 0.04141709263183366
	expectedDelta := -0.4792914536840832
	expectedGamma := 0.0003835840907444611
	expectedTheta := -29.556993293665666
	expectedVega := 551.730541481759
	expectedWeightedVega := 1.
	CheckGreeks(t, optionTheo, expectedTheo, expectedDelta, expectedGamma, expectedTheta, expectedVega, expectedWeightedVega)
}
