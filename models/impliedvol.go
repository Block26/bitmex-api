package models

type ImpliedVol struct {
	Symbol       string  `db:"symbol"`
	IV           float64 `db:"iv"`
	Timestamp    int     `db:"timestamp"`
	Interval     string  `db:"interval"`
	IndexPrice   float64 `db:"indexprice"`
	VWIV         float64 `db:"vwiv"`
	Strike       float64 `db:"strike"`
	TimeToExpiry float64 `db:"timetoexpiry"`
	Volume       float64 `db:"volume"`
}
