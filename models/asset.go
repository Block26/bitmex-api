package models

// The Asset struct is meant to map an asset's symbol (i.e. XBT) to the current quantity of that asset (i.e. 1)
type Asset struct {
	Symbol   string
	Quantity float64
}
