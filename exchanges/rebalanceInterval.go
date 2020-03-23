package exchanges

type rebalanceInterval struct {
	Minute string
	Hour   string
}

// RebalanceInterval set the base definitions for rebalance intervals
func RebalanceInterval() rebalanceInterval {
	r := rebalanceInterval{}
	r.Minute = "1m"
	r.Hour = "1h"
	return r
}
