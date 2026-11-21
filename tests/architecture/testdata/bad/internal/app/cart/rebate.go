// Package cart: a domain file that imports a use case and a helper (GOARCH-9).
package cart

import (
	"example.com/app/internal/app/order/features/place"
	"example.com/app/internal/app/order/features/place/pricing"
)

var _ = place.Decide

var _ = pricing.Rate
