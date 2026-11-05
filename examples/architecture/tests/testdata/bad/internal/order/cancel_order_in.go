package order

import "example.com/app/internal/stock"

type CancelOrderRequest struct{}

type CancelOrderResponse struct{}

// CancelOrderPort takes no context (GOARCH-33); the adapter implements Close by promotion and lacks Cancel
// (GOARCH-5).
type CancelOrderPort interface {
	Cancel()
	Close()
}

// CancelOrderCommand uses another use case of its package and a use case of stock (GOARCH-10); the use case has
// no logic file (GOARCH-3), no document (GOARCH-14) and no tests (GOARCH-34).
func CancelOrderCommand() {
	_ = PlaceOrderAdapter{}
	_ = stock.CountStock()
	CancelOrderAdapter{}.Refund() // the entry calls the adapter (GOARCH-32)
	l := Line{SKU: "c"}
	if l.Valid() != nil {
		return
	}
	l.SKU = "d" // a use case assigns a field of the domain (GOARCH-28)
}

// cancelOrderNotify takes an anonymous interface, a second interface of the in file (GOARCH-12).
func cancelOrderNotify(n interface{ Notify() }) {}

// CancelOrderLabel is a method of the domain struct Line declared in a file of a use case (GOARCH-40).
func (l Line) CancelOrderLabel() string { return "" }

// cancelOrderDefault builds a domain value at the top level, where nothing validates it (GOARCH-29).
var cancelOrderDefault = Line{SKU: "default"}

// cancelOrderAgain is a closure that calls the logic (GOARCH-32).
var cancelOrderAgain = func() { CancelOrder() }
