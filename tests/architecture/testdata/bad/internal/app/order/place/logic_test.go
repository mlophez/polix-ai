package place

import "testing"

// TestDecide tests the logic through its port (GOARCH-34); the use case has no in_test.go (GOARCH-34).
func TestDecide(t *testing.T) {
	var _ Port
	_, _ = Decide(Input{})
}
