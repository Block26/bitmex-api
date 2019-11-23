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
			vol = data.IV
			break
		}
	}
	return vol
}

//TODO: Formatting
func GetDeribitOptionSymbol(expiry float64, strike float64, currency string, optionType string) string {
	expiryTime := time.Unix(int64(expiry/1000), 0)
	year, month, day := expiryTime.Date()
	return "BTC-" + string(day) + string(month) + string(year) + "-" + optionType
}

func GetNextFriday(currentTime time.Time) time.Time {
	dayDiff := currentTime.Weekday()
	if dayDiff <= 0 {
		dayDiff += 7
	}
	return currentTime.Truncate(24 * time.Hour).Add(time.Hour * time.Duration(24*dayDiff))
}

func GetLastFridayOfMonth(currentTime time.Time) time.Time {
	year, month, _ := currentTime.Date()
	firstOfMonth := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1).Day()
	for i := lastOfMonth; i > 0; i-- {
		if currentTime.Weekday() == 5 {
			return currentTime
		}
		currentTime = currentTime.Add(-time.Hour * time.Duration(24))
	}
	return currentTime
}

func GetQuarterlyExpiry(currentTime time.Time, minDays int) time.Time {
	year, month, _ := currentTime.Add(time.Hour * time.Duration(24)).Date()
	// Get nearest quarterly month
	quarterlyMonth := month + (month % 4)
	if quarterlyMonth > 12 {
		year += 1
		quarterlyMonth = quarterlyMonth % 12
	}
	return GetLastFridayOfMonth(time.Date(year, month, 1, 0, 0, 0, 0, time.UTC))
}

func AdjustForSlippage(theo models.OptionTheo, premium float64, side string) float64 {
	adjPremium := premium
	if side == "buy" {
		adjPremium = premium * (1 - (Slippage / 100.))
	} else if side == "sell" {
		adjPremium = premium * (1 + (Slippage / 100.))
	}
	return adjPremium
}

func GetExpiredOptions(currentTime int, options *[]models.OptionContract) []*models.OptionContract {
	var expiredOptions []*models.OptionContract
	for _, option := range *options {
		if option.Expiry >= currentTime {
			option.Status = "expired"
			expiredOptions = append(expiredOptions, &option)
		}
	}
	return expiredOptions
}

func AggregateOptionPnl(options *[]models.OptionContract, currentTime int, currentPrice float64) {
	for _, option := range GetExpiredOptions(currentTime, options) {
		option.Profit = option.Position * (option.OptionTheo.getExpiryValue(currentPrice) - option.AverageCost)
		option.Position = 0
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
				symbol := GetDeribitOptionSymbol(float64(expiry), strike, "BTC", optionType)
				optionContract := models.OptionContract(symbol, strike, expiry, optionType, TickSize, MakerFee,
					TakerFee, MinimumOrderSize, orderArray, orderArray, 0., optionTheo)
				optionContracts = append(optionContracts, optionContract)
			}
		}
	}
	return optionContracts
}
