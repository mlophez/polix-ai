// Package cart: a domain file that imports a use case and a helper (GOARCH-9).
package cart

import (
	"example.com/app/internal/app/order/place"
	"example.com/app/internal/app/order/place/pricing"
)

var _ = place.Decide

var _ = pricing.Rate
