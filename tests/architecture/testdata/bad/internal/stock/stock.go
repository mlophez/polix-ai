// Package stock imports order, which imports stock (GOARCH-7).
package stock

import "example.com/app/internal/order"

// Stock is not a struct (GOARCH-2).
type Stock int

const Level = 1

// stockLimit is a constant the shell of count_stock decides on (GOARCH-37).
const stockLimit = true

var _ = order.Item{}

// StockPort is named as the port of a use case, so it is never a domain type, even in a domain file (GOARCH-23,
// GOARCH-33).
type StockPort struct{}

func (p StockPort) Valid() error { return nil }
