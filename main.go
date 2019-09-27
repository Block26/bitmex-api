//export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
//go install github.com/block26/TheAlgoV2 && github.com/block26/TheAlgoV2
// export GPG_TTY=$(tty)

package main

import (
	"log"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "live" {
			connect("settings/sample_config.json", false)
			// connect("dev/mm/testnet", true)
		} else {
			log.Println("RUN A BACKTEST")
			runBacktest()
		}
	} else {
		log.Println("RUN A BACKTEST")
		runBacktest()
	}
}
