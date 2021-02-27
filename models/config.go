package models

import (
	"encoding/json"
	"io/ioutil"
)

// The Config struct contains metadata for an algorithm including its live cloud environment.
type Config struct {
	Algo          string `json:"algo"`
	Name          string `json:"name"`
	Exchange      string `json:"exchange"`
	Symbol        string `json:"symbol"`
	Secret        string `json:"secret"`
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
	ClusterName   string `json:"cluster_name"`
	SecurityGroup string `json:"security_group"`
	PaperTrade    bool   `json:"paper_trade"`
	SkipTests     bool   `json:"skip_tests"`
	Subnet        string `json:"subnet"`
	RegionName    string `json:"region_name"`
	YantraVersion string `json:"yantra_version"`
	AccountID     int    `json:"account_id"`
	Status        int    `json:"status"`
	UpdatedAt     int64  `json:"updated_at"`
}

// Loads a config from a file.
func LoadConfig(fileName string) (config Config) {
	file, _ := ioutil.ReadFile(fileName)
	_ = json.Unmarshal([]byte(file), &config)
	return
}
