// Package stock imports order, which imports stock (GOARCH-7).
package stock

import "example.com/app/internal/order"

type Stock struct{ Level int }

const Level = 1

var _ = order.Item{}
