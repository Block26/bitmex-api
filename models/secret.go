package models

type Secret struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}
