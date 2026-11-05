// Package stock imports order, which imports stock (GOARCH-7).
package stock

import "example.com/app/internal/order"

// Stock is not a struct (GOARCH-2).
type Stock int

const Level = 1

var _ = order.Item{}
