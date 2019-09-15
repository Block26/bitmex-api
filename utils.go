package main

import (
    "math"
    "os"
    "log"
    "encoding/json"
    "GoMarketMaker/models"
)

func loadConfiguration(file string, secret bool) models.Config {
    var config models.Config
    if secret {
        secret := getSecret(file)
        config = models.Config{}
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

func reverseArr(a []float64) []float64 {
	for i := len(a)/2-1; i >= 0; i-- {
		opp := len(a)-1-i
		a[i], a[opp] = a[opp], a[i]
	}
	return a
}

func arange(min float64, max float64, step float64) []float64 {
    a := make([]float64, int32((max-min)/step)+1)
    for i := range a {
        a[i] = float64(int32(min)) + (float64(i) * step)
    }
    return a
}

func expArr(arr []float64, exp float64) []float64 {
    a := make([]float64, len(arr))
    for i := range arr {
        a[i] = exponent(arr[i], exp)-1
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

func exponent(x, y float64 ) float64 {
	return math.Pow(x, y)
}