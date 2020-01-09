//export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
//go install github.com/tantralabs/yantra && yantra
// export GPG_TTY=$(tty)

package algo

import (
	"github.com/tantralabs/yantra/models"
)

var commitHash string

type Algo struct {
	//Required
	Name   string
	Market models.Market

	FillType            string
	DecisionInterval    string
	LeverageTarget      float64
	EntryOrderSize      float64
	ExitOrderSize       float64
	DeleverageOrderSize float64
	AutoOrderPlacement  bool
	CanBuyBasedOnMax    bool
	LogBacktestToCSV    bool

	State map[string]interface{}

	Debug      bool
	Index      int
	Timestamp  string
	DataLength int
	History    []models.History
	Params     map[string]interface{}
	Result     map[string]interface{}
}
