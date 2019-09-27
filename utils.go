package main

import (
	"encoding/json"
	"log"
	"math"
	"os"

	"github.com/block26/TheAlgoV2/settings"
	"github.com/block26/exchanges/models"
)

func loadConfiguration(file string, secret bool) settings.Config {
	var config settings.Config
	if secret {
		secret := getSecret(file)
		config = settings.Config{}
		json.Unmarshal([]byte(secret), &config)
		return config
	} else {
		configFile, err := os.Open(file)
		defer configFile.Close()
		if err != nil {
			log.Println(err.Error())
		}
		jsonParser := json.NewDecoder(configFile)
		jsonParser.Decode(&config)
		return config
	}
}

func createSpread(weight int32, confidence float64, price float64, spread float64, tickSize float64, maxOrders int32) models.OrderArray {
	xStart := 0.0
	if weight == 1 {
		xStart = price - (price * spread)
	} else {
		xStart = price
	}

	xEnd := xStart + (xStart * spread)
	diff := xEnd - xStart

	if diff/tickSize >= float64(maxOrders) {
		tickSize = diff / (float64(maxOrders) - 1)
	}

	priceArr := arange(xStart, xEnd, float64(int32(tickSize)))
	temp := divArr(priceArr, xStart)
	// temp := (priceArr/xStart)-1

	dist := expArr(temp, confidence)

	normalizer := 1 / sumArr(dist)
	orderArr := mulArr(dist, normalizer)
	if weight == 1 {
		orderArr = reverseArr(orderArr)
	}

	return models.OrderArray{Price: priceArr, Quantity: orderArr}
}

func reverseArr(a []float64) []float64 {
	for i := len(a)/2 - 1; i >= 0; i-- {
		opp := len(a) - 1 - i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func arange(min float64, max float64, step float64) []float64 {
	a := make([]float64, int32((max-min)/step)+1)
	for i := range a {
		a[i] = float64(int32(min+step)) + (float64(i) * step)
	}
	return a
}

func expArr(arr []float64, exp float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = exponent(arr[i]-1, exp)
	}
	return a
}

func mulArrs(a []float64, b []float64) []float64 {
	n := make([]float64, len(a))
	for i := range a {
		n[i] = a[i] * b[i]
	}
	return n
}

func mulArr(arr []float64, multiple float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = float64(arr[i]) * multiple
	}
	return a
}

func divArr(arr []float64, divisor float64) []float64 {
	a := make([]float64, len(arr))
	for i := range arr {
		a[i] = float64(arr[i]) / divisor
	}
	return a
}

func sumArr(arr []float64) float64 {
	sum := 0.0
	for i := range arr {
		sum = sum + arr[i]
	}
	return sum
}

func exponent(x, y float64) float64 {
	return math.Pow(x, y)
}
