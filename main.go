//export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
//go install github.com/tantralabs/TheAlgoV2 && TheAlgoV2
// export GPG_TTY=$(tty)

package algo

import (
	algoModels "github.com/tantralabs/TheAlgoV2/models"
	"github.com/tantralabs/exchanges/models"
)

var commitHash string

type Asset struct {
	Symbol   string
	Quantity float64
}

type OptionContract struct {
	Symbol string
}

type Market struct {
	Symbol           string
	Exchange         string
	ExchangeURL      string
	WSStream         string
	BaseAsset        Asset
	QuoteAsset       Asset
	MaxOrders        int32
	Weight           int32
	Price            float64
	Profit           float64
	AverageCost      float64
	TickSize         float64
	MakerFee         float64
	TakerFee         float64
	MinimumOrderSize float64
	Buying           float64
	Selling          float64
	Leverage         float64
	BuyOrders        models.OrderArray
	SellOrders       models.OrderArray

	Futures bool
	Options []OptionContract
}

type Algo struct {
	//Required
	Name   string
	Market Market

	FillType string

	State map[string]interface{}

	Debug      bool
	Index      int
	DataLength int
	History    []algoModels.History
	Params     map[string]interface{}
	Result     map[string]interface{}
}

// Example
// func main() {
// 	algo := Algo{
// 		Asset: Asset{
// 			BaseBalance: 1.0,
// 			Quantity:    0,
// 			AverageCost: 0.0,
// 			MaxOrders:   15,
// 			MaxLeverage: 0.2,
// 			TickSize:    2,
// 		},
// 		Debug:           true,
// 		Futures:         true,
// 		EntrySpread:     0.05,
// 		EntryConfidence: 1,
// 		ExitSpread:      0.01,
// 		ExitConfidence:  1,
// 		Liquidity:       0.1,
// 	}

// 	if len(os.Args) > 1 {
// 		if os.Args[1] == "live" {
// 			// algo.Connect("settings/sample_config.json", false, algo)
// 			// algo.connect("dev/mm/testnet", true)
// 		} else {
// 			log.Println("RUN A BACKTEST")
// 			algo.RunBacktest()
// 		}
// 	} else {
// 		log.Println("RUN A BACKTEST")
// 		algo.RunBacktest()
// 	}
// }
