// Package stock: a test of the domain imports a use case (GOARCH-9).
package stock

import "example.com/app/internal/app/order/cancel"

var _ = cancel.Command
