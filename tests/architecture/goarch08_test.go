package architecture

import "testing"

// GOARCH-8 allows, it does not forbid: a use case may import the domain of any package. What it leaves out, using
// the use cases of another package, is checked by GOARCH-9 and GOARCH-10.
func TestGOARCH8(t *testing.T) {
	t.Skip("a permission, checked through GOARCH-9 and GOARCH-10")
}
