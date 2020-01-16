package models

type Market struct {
	Symbol           string
	Exchange         string
	ExchangeURL      string
	WSStream         string
	BaseAsset        Asset
	QuoteAsset       Asset
	MaxOrders        int
	Weight           int
	Price            Bar
	Profit           float64
	AverageCost      float64
	TickSize         float64
	MakerFee         float64
	TakerFee         float64
	MinimumOrderSize float64
	Buying           float64
	Selling          float64
	Leverage         float64
	MaxLeverage      float64
	BuyOrders        OrderArray
	SellOrders       OrderArray

	QuantityPrecision    int
	QuantityTickSize     int
	PricePrecision       int
	Futures              bool
	Options              bool
	BulkCancelSupported  bool
	OptionContracts      []OptionContract
	OptionStrikeInterval float64
	OptionTickSize       float64
	OptionMinOrderSize   float64
	OptionMaxStrikePct   float64
	OptionMinStrikePct   float64
	NumWeeklyOptions     int
	NumMonthlyOptions    int
}
