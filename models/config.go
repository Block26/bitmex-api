package models

type Config struct {
    Symbol string `json:"symbol"`
    MaxOrders int32 `json:"max_orders"`
    MaxRetries int32 `json:"max_retries"`
    ApiKey string `json:"api_key"`
    ApiSecret string `json:"api_secret"`
    TestNet bool `json:"test_net"`
}