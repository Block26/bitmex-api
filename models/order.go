package models

type Order struct {
    Symbol string 		`json:"symbol"`
	ClOrdID string		`json:"clOrdID"`
	OrdType string  	`json:"ordType"`
	Price float64		`json:"price"`
	Side string			`json:"side"`
	OrderQty int32		`json:"orderQty"`
	ExecInst string		`json:"execInst"`
	OrigClOrdID string  `json:"origClOrdID"`
}