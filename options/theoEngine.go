package options

import (
	"math"

	"github.com/tantralabs/yantra/logger"
	"github.com/tantralabs/yantra/models"
)

type TheoEngine struct {
	Currency        string
	Options         []models.OptionContract
	UnderlyingPrice float64
	CurrentTime     int
	BaseVolatility  float64
	VolData         []models.ImpliedVol
	backtest        bool
}

const DefaultVolatility = .6

func GetExpiredOptions(currentTime int, options []*models.OptionContract) []*models.OptionContract {
	var expiredOptions []*models.OptionContract
	for _, option := range options {
		if option.Expiry <= currentTime && option.Status != "expired" {
			// logger.Debugf("Found expired option %v at time %v\n", option.OptionTheo.String(), currentTime)
			option.Status = "expired"
			expiredOptions = append(expiredOptions, option)
		}
	}
	return expiredOptions
}

func GetOpenOptions(options []*models.OptionContract) []*models.OptionContract {
	var openOptions []*models.OptionContract
	for _, option := range options {
		if option.Status == "open" {
			openOptions = append(openOptions, option)
		}
	}
	return openOptions
}

func SetMidMarketVols(options []*models.OptionContract) {
	for _, option := range options {
		if option.MidMarketPrice > 0 {
			option.OptionTheo.CalcVol(option.MidMarketPrice)
		}
	}
}

func PropagateVolatility(options []*models.OptionContract, defaultVolatility float64) {
	logger.Debugf("Propagating volatility for %v options with default volatility %v\n", len(options), defaultVolatility)
	expiryToVol := map[int]float64{}
	for _, option := range options {
		if option.OptionTheo.Volatility > 0 {
			if _, ok := expiryToVol[option.Expiry]; !ok {
				expiryToVol[option.Expiry] = option.OptionTheo.Volatility
			}
		}
	}
	for _, option := range options {
		if vol, ok := expiryToVol[option.Expiry]; ok {
			if !math.IsNaN(vol) {
				option.OptionTheo.Volatility = vol
			} else {
				option.OptionTheo.Volatility = defaultVolatility
			}

		} else {
			option.OptionTheo.Volatility = defaultVolatility
		}
	}
}

func AggregateExpiredOptionPnl(options []*models.OptionContract, currentTime int, currentPrice float64) {
	logger.Debugf("Aggregating expired option PNL at %v\n", currentTime)
	PropagateVolatility(options, DefaultVolatility)
	for _, option := range options {
		if option.OptionTheo.Volatility < 0 {
			logger.Debugf("Found option with negative volatitlity after propagation: %v\n", option.Symbol)
		}
	}
	for _, option := range GetExpiredOptions(currentTime, options) {
		if option.AverageCost != 0 {
			option.Profit = option.Position * (option.OptionTheo.GetExpiryValue(currentPrice) - option.AverageCost)
			logger.Debugf("Aggregated profit at price %v for %v with position %v: %v\n", currentPrice, option.OptionTheo.String(), option.Position, option.Profit)
			option.Position = 0
		}
	}
}

func AggregateOpenOptionPnl(options []*models.OptionContract, currentTime int, currentPrice float64, method string) {
	logger.Debugf("Aggregating open option PNL at %v\n", currentTime)
	for i := range options {
		option := options[i]
		if option.Position != 0 {
			logger.Debugf("Found option %v with position %v\n", option.Symbol, option.Position)
			option.OptionTheo.CurrentTime = currentTime
			option.OptionTheo.UnderlyingPrice = currentPrice
			theo := 0.
			if method == "BlackScholes" {
				option.OptionTheo.CalcBlackScholesTheo(true)
				theo = option.OptionTheo.Theo
			} else if method == "BinomialTree" {
				option.OptionTheo.CalcBinomialTreeTheo(.5, 15)
				theo = option.OptionTheo.BinomialTheo
			}
			if theo >= 0 {
				option.Profit = option.Position * (theo - option.AverageCost)
				logger.Debugf("[%v] calced profit: %v with avgcost %v, current theo %v, position %v\n", option.Symbol, option.Profit, option.AverageCost, option.OptionTheo.Theo, option.Position)
			} else {
				logger.Debugf("[%v] Cannot calculate profit for option with negative theo %v\n", option.Symbol, theo)
			}
		}
	}
}
