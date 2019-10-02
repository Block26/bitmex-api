//export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
//go install github.com/block26/TheAlgoV2 && github.com/block26/TheAlgoV2
// export GPG_TTY=$(tty)

package main

import (
	"log"
	"os"
)

func main() {
	algo := Algo{
		Asset: Asset{
			BaseBalance: 1.0,
			Quantity:    0,
			AverageCost: 0.0,
			MaxOrders:   15,
			MaxLeverage: 0.2,
			TickSize:    2,
		},
		Debug:           true,
		Futures:         true,
		EntrySpread:     0.05,
		EntryConfidence: 1,
		ExitSpread:      0.01,
		ExitConfidence:  1,
		Liquidity:       0.1,
	}

	if len(os.Args) > 1 {
		if os.Args[1] == "live" {
			algo.connect("settings/sample_config.json", false)
			// connect("dev/mm/testnet", true)
		} else {
			log.Println("RUN A BACKTEST")
			algo.runBacktest()
		}
	} else {
		log.Println("RUN A BACKTEST")
		algo.runBacktest()
	}
}
