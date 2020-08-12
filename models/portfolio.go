package models

import (
	"encoding/json"
	"io/ioutil"
)

type Portfolio struct {
	Algos map[string]PortfolioAlgo
}

func (p *Portfolio) AlgoNames() (names []string) {
	for name := range p.Algos {
		names = append(names, name)
	}
	return
}

type PortfolioAlgo struct {
	Weight float64 `json:"weight"`
}

// Loads a config from a file.
func LoadPortfolio(fileName string) (portfolio Portfolio) {
	file, _ := ioutil.ReadFile(fileName)
	_ = json.Unmarshal([]byte(file), &portfolio.Algos)
	return
}
