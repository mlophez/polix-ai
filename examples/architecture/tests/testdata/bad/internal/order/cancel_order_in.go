package order

import "example.com/app/internal/stock"

type CancelOrderRequest struct{}

type CancelOrderResponse struct{}

type CancelOrderPort interface{ Cancel() }

// CancelOrderCommand uses another use case of its package and a use case of stock (GOARCH-10); the use case has
// no logic file (GOARCH-3) nor document (GOARCH-14).
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
