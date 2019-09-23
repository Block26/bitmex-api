package settings

type Config struct {
	Symbol     string `json:"symbol"`
	MaxOrders  int32  `json:"max_orders"`
	MaxRetries int32  `json:"max_retries"`
	APIKey     string `json:"api_key"`
	APISecret  string `json:"api_secret"`
	TestNet    bool   `json:"test_net"`
}
