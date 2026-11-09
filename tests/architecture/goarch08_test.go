package architecture

import "testing"

// GOARCH-8 allows, it does not forbid: a use case may import the domain of any noun. What it leaves out is the
// direction of the imports, checked by GOARCH-9.
func TestGOARCH8(t *testing.T) {
	t.Skip("a permission, checked through GOARCH-9")
}
