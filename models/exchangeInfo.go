package models

type ExchangeInfo struct {
	Exchange  string
	IsTestnet bool

	// These are also configurable at the market level, but we can set defaults here for convenience
	ExchangeURL             string
	WSStream                string
	MaxOrders               int
	MakerFee                float64
	TakerFee                float64
	Slippage                float64
	PricePrecision          float64
	QuantityPrecision       float64
	MinimumOrderSize        float64
	MaxLeverage             float64
	BulkCancelSupported     bool
	DenominatedInUnderlying bool

	// Supported symbols for the exchange
	Symbols map[string]bool

	// Indicate whether each of these is supported
	Spot    bool
	Futures bool
	Options bool

	// Options configs
	NumWeeklyOptions       int
	NumMonthlyOptions      int
	OptionMinStrikePct     float64
	OptionMaxStrikePct     float64
	OptionStrikeInterval   float64
	OptionMinimumOrderSize float64
	OptionSlippage         float64
	OptionMakerFee         float64
	OptionTakerFee         float64
}
