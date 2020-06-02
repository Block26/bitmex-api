package models

// Account history is meant to give a precise summary of the current account state for insertion into the db.
type AccountHistory struct {
	Timestamp        int
	UnrealizedProfit float64
	RealizedProfit   float64
	Profit           float64
}

// Constructs a new account history struct given an account and current timestamp.
func NewAccountHistory(account Account, timestamp int) AccountHistory {
	return AccountHistory{
		Timestamp:        timestamp,
		UnrealizedProfit: account.UnrealizedProfit,
		RealizedProfit:   account.RealizedProfit,
		Profit:           account.Profit,
	}
}
