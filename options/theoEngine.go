package options

import (
	"fmt"
	"math"
	"time"

	base "github.com/tantralabs/TheAlgoV2"
	"github.com/tantralabs/TheAlgoV2/models"
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

// Get options available at a given time (assume a given NumWeekly and NumMonthly available)
const NumWeekly = 2
const NumMonthly = 3
const NumStrikes = 10
const StrikeInterval = 250.
const TickSize = .1

//TODO: Inspect deribit options trading fees
const MakerFee = .0004
const TakerFee = .0004
const MinimumOrderSize = .1

// Assume Slippage percent loss on market orders
const Slippage = 5.
const MaxProfitPct = 50.
const MaxLossPct = 50.

func (t *TheoEngine) getOptions(backtest bool) *[]models.OptionContract {
	if backtest {
		t.Options = BuildAvailableOptions(t.UnderlyingPrice, base.TimestampToTime(t.CurrentTime), t.BaseVolatility)
	} else {
		fmt.Printf("Getting options from exchange not yet implemented")
	}
	return &t.Options
}

func GetNearestVol(volData []models.ImpliedVol, time int) float64 {
	vol := -1.
	for _, data := range volData {
		timeDiff := time - data.Timestamp
		if timeDiff < 0 {
			vol = data.IV / 100 //Assume volData quotes IV in pct
			break
		}
	}
	return vol
}

func GetExpiredOptions(currentTime int, options []*models.OptionContract) []*models.OptionContract {
	var expiredOptions []*models.OptionContract
	for _, option := range options {
		if option.Expiry <= currentTime && option.Status != "expired" {
			// fmt.Printf("Found expired option %v at time %v\n", option.OptionTheo.String(), currentTime)
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

func AggregateExpiredOptionPnl(options []*models.OptionContract, currentTime int, currentPrice float64) {
	for _, option := range GetExpiredOptions(currentTime, options) {
		option.Profit = option.Position * (option.OptionTheo.GetExpiryValue(currentPrice) - option.AverageCost)
		// fmt.Printf("Aggregated profit at price %v for %v with position %v: %v\n", currentPrice, option.OptionTheo.String(), option.Position, option.Profit)
		option.Position = 0
	}
}

func AggregateOpenOptionPnl(options []*models.OptionContract, currentTime int, currentPrice float64, method string) {
	for _, option := range options {
		if option.Position != 0 {
			option.OptionTheo.CurrentTime = currentTime
			option.OptionTheo.UnderlyingPrice = currentPrice
			theo := 0.
			if method == "BlackScholes" {
				option.OptionTheo.CalcBlackScholesTheo(false)
				theo = option.OptionTheo.Theo
			} else if method == "BinomialTree" {
				option.OptionTheo.CalcBinomialTreeTheo(.5, 15)
				theo = option.OptionTheo.BinomialTheo
			}
			option.Profit = option.Position * (theo - option.AverageCost)
			// fmt.Printf("Calculated profit for option %v: %v with position %v and theo %v, vol %v, timeToExpiry %v, underlying %v\n", option.OptionTheo.String(), option.Profit, option.Position, option.OptionTheo.Theo, option.OptionTheo.Volatility, option.OptionTheo.TimeLeft, option.OptionTheo.UnderlyingPrice)
		}
	}
}

func BuildAvailableOptions(underlyingPrice float64, currentTime time.Time, volatility float64) []models.OptionContract {
	// Get expiries
	var expirys []int
	nextFriday := GetNextFriday(currentTime)
	for i := 0; i < NumWeekly; i++ {
		expirys = append(expirys, int(nextFriday.UnixNano()/int64(time.Millisecond)))
		nextFriday = nextFriday.Add(time.Hour * time.Duration(24*7))
	}
	year, month, day := currentTime.Date()
	for i := 0; i < NumMonthly; i++ {
		expirys = append(expirys, int(time.Date(year, month, day, 0, 0, 0, 0, time.UTC).UnixNano()/int64(time.Millisecond)))
		month++
		month = month % 12
	}
	// Get strikes
	midStrike := base.RoundToNearest(underlyingPrice, StrikeInterval)
	minStrike := midStrike - (StrikeInterval * math.Floor(NumStrikes/2))
	maxStrike := midStrike + (StrikeInterval * math.Ceil(NumStrikes/2))
	strikes := base.Arange(minStrike, maxStrike, StrikeInterval)
	// Generate options contracts
	var optionContracts []models.OptionContract
	var orderArray models.OrderArray
	for _, expiry := range expirys {
		for _, strike := range strikes {
			for _, optionType := range []string{"call", "put"} {
				optionTheo := models.NewOptionTheo(optionType, underlyingPrice, strike, int(currentTime.UnixNano()/int64(time.Millisecond)), expiry, 0, volatility, -1)
				symbol := GetDeribitOptionSymbol(expiry, strike, "BTC", optionType)
				optionContract := models.OptionContract{symbol, strike, expiry, optionType, 0, 0, TickSize, MakerFee,
					TakerFee, MinimumOrderSize, orderArray, orderArray, 0., *optionTheo, "open"}
				optionContracts = append(optionContracts, optionContract)
			}
		}
	}
	return optionContracts
}
