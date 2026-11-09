// Package tax is a helper of the use case place that imports its sibling helper pricing (GOARCH-9).
package tax

import "example.com/app/internal/app/order/place/pricing"

var _ = pricing.Rate
