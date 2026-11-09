package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-38: the tests are white box: every *_test.go of internal/ declares the package of its folder, never
// <name>_test.
func TestGOARCH38(t *testing.T) {
	checkRule(t, checkWhiteBoxTests,
		"internal/app/order/order_test.go: declares package order_test, not order: the tests are white box")
}

func checkWhiteBoxTests(m *module) []string {
	var out []string
	for _, f := range m.files {
		if f.place.level == levelCmd || f.kind != kindTest {
			continue
		}
		if f.ast.Name.Name != f.pkg {
			out = append(out, fmt.Sprintf("%s: declares package %s, not %s: the tests are white box", f.rel,
				f.ast.Name.Name, f.pkg))
		}
	}
	return out
}
