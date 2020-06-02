package models

// Contains a slice of prices and quantities that line up with a series of orders.
type OrderArray struct {
	Price    []float64
	Quantity []float64
}
