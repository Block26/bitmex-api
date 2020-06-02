package models

// Summarizes the state of a market at a given point in time, including PNL statistics.
type MarketHistory struct {
	Timestamp        int
	Symbol           string
	MarketType       MarketType
	Balance          float64
	AverageCost      float64
	UnrealizedProfit float64
	RealizedProfit   float64
	Position         float64
	Open             float64
	High             float64
	Low              float64
	Close            float64
	Volume           float64
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

// Constructs new market history state given a timestamp.
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
	// ohlcv, index := market.OHLCV.GetOHLCVData(1)
	if market.OHLCV != nil {
		// logger.Errorf("Close: %v\n", ohlcv.Close[index])
		// lastBar := *barData[len(barData)-1]
		// logger.Errorf("Last bar: %v, first bar: %v\n", lastBar, *barData[0])
		// for _, bar := range barData {
		// 	logger.Errorf("%v, ", bar)
		// }
		// logger.Errorf("\n")
		return MarketHistory{
			Timestamp:        timestamp,
			Symbol:           market.Symbol,
			MarketType:       market.Info.MarketType,
			Balance:          market.Balance,
			AverageCost:      market.AverageCost,
			UnrealizedProfit: market.UnrealizedProfit,
			RealizedProfit:   market.RealizedProfit,
			Position:         market.Position,
			// Open:             ohlcv.Open[index],
			// High:             ohlcv.High[index],
			// Low:              ohlcv.Low[index],
			// Close:            ohlcv.Close[index],
			// Volume:           ohlcv.Volume[index],
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
