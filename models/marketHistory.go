package models

type MarketHistory struct {
	Timestamp        int
	Symbol           string
	MarketType       MarketType
	Balance          float64
	AverageCost      float64
	UnrealizedProfit float64
	RealizedProfit   float64
	Position         float64
	Strike           float64
	Expiry           int
	OptionType       OptionType
	Theo             float64
	Delta            float64
	Gamma            float64
	Theta            float64
	Vega             float64
	WeightedVega     float64
	Volatility       float64
}

func NewMarketHistory(market MarketState, timestamp int) MarketHistory {
	if market.Info.MarketType == Option {
		return MarketHistory{
			Timestamp:        timestamp,
			Symbol:           market.Symbol,
			MarketType:       market.Info.MarketType,
			Balance:          market.Balance,
			AverageCost:      market.AverageCost,
			UnrealizedProfit: market.UnrealizedProfit,
			RealizedProfit:   market.RealizedProfit,
			Position:         market.Position,
			Strike:           market.Info.Strike,
			Expiry:           market.Info.Expiry,
			OptionType:       market.Info.OptionType,
			Theo:             market.OptionTheo.Theo,
			Delta:            market.OptionTheo.Delta,
			Gamma:            market.OptionTheo.Gamma,
			Theta:            market.OptionTheo.Theta,
			Vega:             market.OptionTheo.Vega,
			WeightedVega:     market.OptionTheo.WeightedVega,
			Volatility:       market.OptionTheo.Volatility,
		}
	}
	return MarketHistory{
		Timestamp:        timestamp,
		Symbol:           market.Symbol,
		MarketType:       market.Info.MarketType,
		Balance:          market.Balance,
		AverageCost:      market.AverageCost,
		UnrealizedProfit: market.UnrealizedProfit,
		RealizedProfit:   market.RealizedProfit,
		Position:         market.Position,
	}
}
