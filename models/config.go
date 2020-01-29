package models

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Name     string `json: "name"`
	Exchange string `json: "exchange"`
	Symbol   string `json: "symbol"`
	Secret   string `json: "secret"`
}

func Load(fileName string) (config Config) {
	file, _ := ioutil.ReadFile(fileName)
	_ = json.Unmarshal([]byte(file), &config)
	return
}
