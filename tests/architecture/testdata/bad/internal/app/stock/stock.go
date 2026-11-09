// Package stock imports order, which imports stock (GOARCH-7).
package stock

import "example.com/app/internal/app/order"

// Stock is not a struct (GOARCH-2).
type Stock int

const Level = 1

// Limit is a constant the shell of count decides on (GOARCH-37).
const Limit = true

var _ = order.Item{}

// StockPort is named as the port of a use case, a name a domain never declares and never a domain type
// (GOARCH-23, GOARCH-33).
type StockPort struct{}

func (p StockPort) Valid() error { return nil }
