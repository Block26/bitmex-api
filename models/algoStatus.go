package models

type AlgoStatus struct {
	Leverage           float64 `json:"leverage"`
	ShouldHaveLeverage float64 `json:"should_have_leverage"`
}
