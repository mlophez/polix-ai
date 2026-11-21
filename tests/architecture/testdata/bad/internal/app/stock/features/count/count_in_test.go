package count

import "testing"

// fakePort is the fake of the port, which the test of the logic must not use (GOARCH-34).
type fakePort struct{}

// TestRun does not call the shell run and mentions the adapter (GOARCH-34).
func TestRun(t *testing.T) {
	_ = Adapter{}
	_ = fakePort{}
}
