package architecture

import (
	"fmt"
	"strings"
	"testing"
)

// GOARCH-39: no dot import in internal/, pkg/ nor cmd/: every name of another package is written with its
// qualifier.
func TestGOARCH39(t *testing.T) {
	checkRule(t, checkNoDotImport, "internal/order/order.go:5: dot imports strings")
}

func checkNoDotImport(m *module) []string {
	var out []string
	for _, f := range m.files {
		for _, s := range f.ast.Imports {
			if s.Name != nil && s.Name.Name == "." {
				out = append(out, fmt.Sprintf("%s: dot imports %s: every name carries the qualifier of its package",
					m.pos(f, s), strings.Trim(s.Path.Value, `"`)))
			}
		}
	}
	return out
}
