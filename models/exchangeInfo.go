package models

// The ExchangeInfo struct contains various information unique to a given exchange.
type ExchangeInfo struct {
	Exchange  string
	IsTestnet bool

	// These are also configurable at the market level, but we can set defaults here for convenience
	ExchangeURL             string  // REST API URL endpoint
	WSStream                string  // Websocket API URL endpoint
	MaxOrders               int     // Maximum outstanding orders that a single account can have on the exchange
	MakerFee                float64 // Default fees incurred on maker order as decimal (i.e. 1% = .01)
	TakerFee                float64 // Default fees incurred on taker order as decimal (i.e. 1% = .01)
	Slippage                float64 // Default slippage incurred on taker order as decimal (i.e. 1% = .01)
	PricePrecision          float64 // Smallest amount by which price can vary
	QuantityPrecision       float64 // Smallest amount by which quantity can vary
	MinimumOrderSize        float64 // Smallest possible order amount
	MaxLeverage             float64 // Maximum leverage offered by the exchange
	BulkCancelSupported     bool    // Does this exchange allow us to cancel multiple orders with one API call?
	DenominatedInUnderlying bool    // Are prices on this exchange denominated in the base asset?
	DenominatedInQuote      bool    // Are the prices on this exchange denominated in the quote asset?

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
