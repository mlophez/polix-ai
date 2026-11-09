package cancel

import (
	"example.com/app/internal/app/order"
	"example.com/app/internal/app/order/place"
	"example.com/app/internal/app/order/place/pricing"
	"example.com/app/internal/app/stock/count"
)

type Request struct{}

type Response struct{}

// Port takes no context (GOARCH-33); the adapter implements Close by promotion and lacks Cancel (GOARCH-5).
type Port interface {
	Cancel()
	Close()
}

// Command uses another use case of its noun, a use case of stock and a helper of place (GOARCH-9); the use case
// has no logic file (GOARCH-3), no shell (GOARCH-32), no document (GOARCH-14) and no tests (GOARCH-34).
func Command() {
	_ = place.Adapter{}
	_ = count.Decide
	_ = pricing.Rate()
	Adapter{}.Refund() // the entry calls the adapter (GOARCH-32)
	l := order.Line{SKU: "c"}
	if l.Valid() != nil {
		return
	}
	l.SKU = "d" // a use case assigns a field of the domain (GOARCH-28)
}

// notify takes an anonymous interface, a second interface of the in file (GOARCH-12).
func notify(n interface{ Notify() }) {}

// fallback builds a domain value at the top level, where nothing validates it (GOARCH-29).
var fallback = order.Line{SKU: "default"}

// again is a closure that calls the logic (GOARCH-32).
var again = func() { Decide() }
