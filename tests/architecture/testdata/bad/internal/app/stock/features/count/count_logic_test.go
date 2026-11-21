package count

import "testing"

// TestDecide does not call the logic and uses the fake of the in test (GOARCH-34).
func TestDecide(t *testing.T) {
	_ = fakePort{}
}
