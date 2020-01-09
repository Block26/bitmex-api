package exchanges

type rebalanceInterval struct {
	Minute string
	Hour   string
	Day    string
}

// RebalanceInterval set the base definitions for rebalance intervals
func RebalanceInterval() rebalanceInterval {
	r := rebalanceInterval{}
	r.Minute = "1m"
	r.Hour = "1h"
	r.Day = "1d"
	return r
}
