//export GOPATH=/Users/russell/git/go && export PATH=$PATH:$(go env GOPATH)/bin
//go install github.com/tantralabs/TheAlgoV2 && TheAlgoV2
// export GPG_TTY=$(tty)

package algo

import (
	"github.com/tantralabs/TheAlgoV2/models"
)

var commitHash string

type Algo struct {
	//Required
	Name   string
	Market models.Market

	FillType            string
	LeverageTarget      float64
	EntryOrderSize      float64
	ExitOrderSize       float64
	DeleverageOrderSize float64
	AutoOrderSizing     bool
	CanBuyBasedOnMax    bool

	State map[string]interface{}

	Debug      bool
	Index      int
	Timestamp  string
	DataLength int
	History    []models.History
	Params     map[string]interface{}
	Result     map[string]interface{}
}
