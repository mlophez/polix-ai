package order

import "testing"

// TestPlaceOrder tests the logic through its port (GOARCH-34).
func TestPlaceOrder(t *testing.T) {
	var _ PlaceOrderPort
	_, _ = PlaceOrder(PlaceOrderInput{})
}
