//export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
//go install GoMarketMaker && GoMarketMaker
// export GPG_TTY=$(tty)

package main

import (
	"GoMarketMaker/models"
	"log"
	"os"
)

var settings models.Config

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "live" {
			// connect("settings/sample_config.json", false)
			connect("dev/mm/testnet", true)
		} else {
			log.Println("RUN A BACKTEST")
		}
	} else {
		log.Println("RUN A BACKTEST")
	}
}
