package options

import (
	"fmt"
	"github.com/tantralabs/TheAlgoV2/models"
)


type TheoEngine struct {
	Currency		string
	Options 		[]models.OptionContract
	UnderlyingPrice float64
	CurrentTime     int
	BaseVolatility	float64
	VolData			[]ImpliedVol
	backtest		bool
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

func NewTheoEngine(optionType string, uPrice float64, strike float64,
	currentTime int, expiry int, r float64,
	volatility float64, theo float64) *OptionTheo {
	o := &OptionTheo{
		strike:      strike,
		uPrice:      uPrice,
		r:           r,
		currentTime: currentTime,
		expiry:      expiry,
		timeLeft:    GetTimeLeft(currentTime, expiry),
		optionType:  optionType,
		volatility:  volatility,
		theo:        theo,
	}
	return o
}

func (self *TheoEngine) getOptions(backtest bool) *[]models.OptionContract {
	if backtest {
		self.options = *BuildAvailableOptions(self.UnderlyingPrice, self.CurrentTime, self.BaseVolatility)
	} else {
		fmt.Printf("Getting options from exchange not yet implemented")
	}
	return &self.Options
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
func GetDeribitOptionSymbol(expiry float64, strike float64, currency string, optionType string) {
	expiryTime = time.Unix(int(expiry / 1000))
	year, month, day = expiryTime.Date()
	return "BTC-" + day + month + year + "-" + optionType
}

func GetNextFriday(currentTime time.Time) time.Time {
	dayDiff := currentTime.weekday()
	if dayDiff <= 0 {
		dayDiff += 7
	}
	return currentTime.Truncate(24 * time.Hour).Add(time.Hour * time.Duration(24 * dayDiff))
}

func GetLastFridayOfMonth(year int, month int) time.Time {
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0)
	lastOfMonth := firstOfMonth.AddDate(0, 1, -1)
	for i := lastOfMonth; i > 0; i-- {
		if currentTime.weekday() == 5 {
			return currentTime
		}
		currentTime = currentTime.Add(-time.Hour * time.Duration(24))
	}
	return currentTime
}

func GetQuarterlyExpiry(currentTime time.Time, minDays int) time.Time {
	year, month, day = currentTime.Add(time.Hour * time.Duration(24)).Date()
	// Get nearest quarterly month
	quarterlyMonth = month + (month % 4)
	if quarterlyMonth > 12 {
		year += 1
		quarterlyMonth = quarterlyMonth % 12
	}
	return GetLastFridayOfMonth(year, quarterlyMonth)
}

func AdjustForSlippage(theo OptionTheo, premium float64, side string) float64 {
	adjPremium := premium
	if side == "buy" {
		adjPremium = premium * (1 - (Slippage / 100.))
	} else if side == "sell" {
		adjPremium = premium * (1 + (Slippage / 100.))
	}
	return adjPremium
}

func GetExpiredOptions(currentTime int, options *[]OptionContract) []*OptionContract{
	var expiredOptions []*OptionContract
	for _, option := range options {
		if option.Expiry >= currentTime {
			option.Status = "expired"
			append(expiredOptions, &option)
		}
	}
	return expiredOptions
}

func AggregateOptionPnl(options *[]OptionContract, currentTime int) {
	for _, option := GetExpiredOptions(currentTime, options) {
		option.Profit = option.Position * (option.OptionTheo.getExpiryValue() - option.AverageCost)
		option.Position = 0
	}
}

func BuildAvailableOptions(underlyingPrice float64, currentTime time.Time, volatility float64) []OptionContract{
	// Get expiries
	var expirys []int
	nextFriday = getNextFriday(currentTime)
	for i := 0; i < NumWeekly; i++ {
		expirys = append(expirys, nextFriday.UnixMillis())
		nextFriday = nextFriday.Add(time.Hour * time.Duration(24 * 7))
	}
	year, month, _ = currentTime.Date()
	for i := 0; i < NumMonthly; i++ {
		expirys = append(expirys, getLastFridayOfMonth(month).UnixMillis())
		month++
		month = month % 12
	}
	// Get strikes
	midStrike := roundToNearest(underlyingPrice, StrikeInterval)
	minStrike := midStrike - (StrikeInterval * math.Floor(NumStrikes / 2))
	maxStrike := midStrike + (StrikeInterval * math.Ceil(NumStrikes / 2))
	strikes := arange(minStrike, maxStrike, StrikeInterval)
	// Generate options contracts
	numOptions = (len(expirys)) * NumStrikes
	var optionContracts = []algoModels.OptionContract
	orderArray = : OrderArray[[]float64, []float64]
	for _, expiry := range expirys {
		for _, strike := range strikes {
			for _, optionType := range []string{"call", "put"} {
				optionTheo = NewOptionTheo(optionType, underlyingPrice, strike, currentTime, expiry, 0, volatility, -1)
				symbol := getDeribitOptionSymbol(expiry, strike, "BTC", optionType)
				optionContract := OptionContract(symbol, strike, expiry, optionType, TickSize, MakerFee, 
					TakerFee, MinimumOrderSize, orderArray, orderArray, 0., optionTheo)
				optionContracts = append(optionContracts, optionContract)
			}
		}
	}
	return optionContracts
}