package exchanges

type marketType struct {
	Spot   string
	Future string
	Option string
}

// MarketType set the base definitions for the supported market types
func MarketType() marketType {
	r := marketType{}
	r.Spot = "spot"
	r.Future = "future"
	r.Option = "option"
	return r
}
