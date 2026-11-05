package stock

import "testing"

// TestCountStock does not call the logic and uses the fake of the in test (GOARCH-34).
func TestCountStock(t *testing.T) {
	_ = fakeCountStockPort{}
}
