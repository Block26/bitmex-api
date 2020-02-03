package database

type TransactionRecord struct {
	Id        int     `db:"id"`
	Timestamp int64   `db:"timestamp"`
	Exchange  string  `db:"exchange"`
	Type      string  `db:"type"`
	AccountId string  `db:"account_id"`
	AlgoName  string  `db:"algo_name"`
	TxId      string  `db:"tx_id"`
	Status    string  `db:"status"`
	Currency  string  `db:"currency"`
	Amount    float64 `db:"amount"`
}
