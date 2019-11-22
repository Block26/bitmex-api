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

// Get options available at a given time (assume a given numWeekly and numMonthly available)
const numWeekly = 2
const numMonthly = 3
const numStrikes = 10
const strikeInterval = 250.
const tickSize = .1
//TODO: Inspect deribit options trading fees
const makerFee = .0004
const takerFee = .0004
const minimumOrderSize = .1

func BuildAvailableOptions(underlyingPrice float64, currentTime time.Time, volatility float64) []OptionContract{
	// Get expiries
	var expirys []int
	nextFriday = getNextFriday(currentTime)
	for i := 0; i < numWeekly; i++ {
		expirys = append(expirys, nextFriday.UnixMillis())
		nextFriday = nextFriday.Add(time.Hour * time.Duration(24 * 7))
	}
	year, month, _ = currentTime.Date()
	for i := 0; i < numMonthly; i++ {
		expirys = append(expirys, getLastFridayOfMonth(month).UnixMillis())
		month++
		month = month % 12
	}
	// Get strikes
	midStrike := roundToNearest(underlyingPrice, strikeInterval)
	minStrike := midStrike - (strikeInterval * math.Floor(numStrikes / 2))
	maxStrike := midStrike + (strikeInterval * math.Ceil(numStrikes / 2))
	strikes := arange(minStrike, maxStrike, strikeInterval)
	// Generate options contracts
	numOptions = (len(expirys)) * numStrikes
	var optionContracts = []algoModels.OptionContract
	orderArray = : OrderArray[[]float64, []float64]
	for _, expiry := range expirys {
		for _, strike := range strikes {
			for _, optionType := range []string{"call", "put"} {
				optionTheo = NewOptionTheo(optionType, underlyingPrice, strike, currentTime, expiry, 0, volatility, -1)
				symbol := getDeribitOptionSymbol(expiry, strike, "BTC", optionType)
				optionContract := OptionContract(symbol, strike, expiry, optionType, tickSize, makerFee, 
					takerFee, minimumOrderSize, orderArray, orderArray, 0., optionTheo)
				optionContracts = append(optionContracts, optionContract)
			}
		}
	}
	return optionContracts
}