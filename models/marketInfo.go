package models

type MarketType int

const (
	Future MarketType = iota
	Option
	Spot
)

var marketTypes = [...]string{
	"Future",
	"Option",
	"Spot",
}

type OptionType int

const (
	Call OptionType = iota
	Put
)

var OptionTypes = [...]string{
	"Call",
	"Put",
}

// Stateless market information.
type MarketInfo struct {
	Symbol                  string
	BaseSymbol              string
	QuoteSymbol             string
	MarketType              MarketType
	Exchange                string
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

	// Only used for options
	Strike           float64
	Expiry           int
	OptionType       OptionType
	UnderlyingSymbol string
}

func NewMarketInfo(symbol string, exchangeInfo ExchangeInfo) MarketInfo {
	return MarketInfo{
		Symbol:                  symbol,
		Exchange:                exchangeInfo.Exchange,
		ExchangeURL:             exchangeInfo.ExchangeURL,
		WSStream:                exchangeInfo.WSStream,
		MaxOrders:               exchangeInfo.MaxOrders,
		MakerFee:                exchangeInfo.MakerFee,
		TakerFee:                exchangeInfo.TakerFee,
		Slippage:                exchangeInfo.Slippage,
		PricePrecision:          exchangeInfo.PricePrecision,
		QuantityPrecision:       exchangeInfo.QuantityPrecision,
		MinimumOrderSize:        exchangeInfo.MinimumOrderSize,
		MaxLeverage:             exchangeInfo.MaxLeverage,
		BulkCancelSupported:     exchangeInfo.BulkCancelSupported,
		DenominatedInUnderlying: exchangeInfo.DenominatedInUnderlying,
	}
}
