package stock

import "testing"

// fakeCountStockPort is the fake of the port, which the test of the logic must not use (GOARCH-34).
type fakeCountStockPort struct{}

// TestCountStockShell does not call the shell countStock and mentions the adapter (GOARCH-34).
func TestCountStockShell(t *testing.T) {
	_ = CountStockAdapter{}
	_ = fakeCountStockPort{}
}
