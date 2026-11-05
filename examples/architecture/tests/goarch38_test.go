package tests

import (
	"fmt"
	"testing"
)

// GOARCH-38: the tests are white box: every *_test.go of internal/ and pkg/ declares the package of its folder,
// never <noun>_test.
func TestGOARCH38WhiteBoxTests(t *testing.T) {
	checkRule(t, checkWhiteBoxTests,
		"internal/order/order_test.go: declares package order_test, not order: the tests are white box")
}

func checkWhiteBoxTests(m *module) []string {
	var out []string
	for _, f := range m.files {
		if f.top == "cmd" || f.kind != kindTest {
			continue
		}
		if f.ast.Name.Name != f.pkg {
			out = append(out, fmt.Sprintf("%s: declares package %s, not %s: the tests are white box", f.rel,
				f.ast.Name.Name, f.pkg))
		}
	}
	return out
}
