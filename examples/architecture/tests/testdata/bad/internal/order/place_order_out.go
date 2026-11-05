package order

// PlaceOrderAdapter is not a struct (GOARCH-24).
type PlaceOrderAdapter map[string]string

// NewPlaceOrderAdapter is a constructor, a function of the out file (GOARCH-24).
func NewPlaceOrderAdapter() PlaceOrderAdapter { return PlaceOrderAdapter{} }
