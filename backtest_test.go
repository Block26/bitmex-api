package yantra

import (
	"testing"
)

func TestLogBacktest(t *testing.T) {
	algo := setupAlgo()
	logBacktest(algo)
}