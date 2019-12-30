package models

type OptionContract struct {
	Symbol           string
	Strike           float64
	Expiry           int
	OptionType       string
	AverageCost      float64
	Profit           float64
	TickSize         float64
	MakerFee         float64
	TakerFee         float64
	MinimumOrderSize float64
	BuyOrders        OrderArray
	SellOrders       OrderArray
	Position         float64
	OptionTheo       OptionTheo
	Status           string // "open", "expired"
	MidMarketPrice   float64
}
