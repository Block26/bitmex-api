package exchanges

type fillType struct {
	Limit string
	Open  string
	Close string
	Worst string
	Mean  string
}

// FillType set the base definitions for the supported backtest fill types
func FillType() fillType {
	r := fillType{}
	r.Limit = "limit"
	r.Open = "open"
	r.Close = "close"
	r.Worst = "worst"
	r.Worst = "mean"
	return r
}
