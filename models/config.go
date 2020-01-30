package models

import (
	"encoding/json"
	"io/ioutil"
)

type Config struct {
	Algo       string `json:"algo"`
	Name       string `json:"name"`
	Exchange   string `json:"exchange"`
	Symbol     string `json:"symbol"`
	Secret     string `json:"secret"`
	CommitHash string `json:"commit_hash"`
}

func LoadConfig(fileName string) (config Config) {
	file, _ := ioutil.ReadFile(fileName)
	_ = json.Unmarshal([]byte(file), &config)
	return
}
