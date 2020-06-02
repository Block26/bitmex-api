package models

// Represents API key and secret for an exchange's account. Should be loaded from AWS secrets or local file.
type Secret struct {
	APIKey    string `json:"api_key"`
	APISecret string `json:"api_secret"`
}
