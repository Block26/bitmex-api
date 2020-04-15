package models

type AccountHistory struct {
	Timestamp        int
	UnrealizedProfit float64
	RealizedProfit   float64
	Profit           float64
}

func NewAccountHistory(account Account, timestamp int) AccountHistory {
	return AccountHistory{
		Timestamp:        timestamp,
		UnrealizedProfit: account.UnrealizedProfit,
		RealizedProfit:   account.RealizedProfit,
		Profit:           account.Profit,
	}
}
