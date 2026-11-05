package architecture

import (
	"fmt"
	"strings"
	"testing"
)

// GOARCH-31: every top-level name of the files of a use case (types, functions, constants and variables; not
// methods, init nor _) starts with the name of the use case, <Verb><Noun> or <verb><Noun>: the use cases of a
// package share its namespace, and the name tells which use case a symbol belongs to.
func TestGOARCH31(t *testing.T) {
	checkRule(t, checkUseCasePrefix,
		"internal/order/place_order_in.go:8: Notifier does not start with PlaceOrder",
		"internal/order/cancel_order_out.go:10: Timeout does not start with CancelOrder")
}

func checkUseCasePrefix(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		c, lc := camel(uc.stem), lowerCamel(uc.stem)
		for _, s := range useCaseSuffixes {
			f := uc.files[s.kind]
			if f == nil {
				continue // GOARCH-3
			}
			for _, d := range f.ast.Decls {
				for _, id := range topLevelNames(d) {
					if id.Name == "_" || id.Name == "init" || strings.HasPrefix(id.Name, c) || strings.HasPrefix(id.Name, lc) {
						continue
					}
					out = append(out, fmt.Sprintf("%s: %s does not start with %s or %s: the names of a use case carry "+
						"its name", m.pos(f, id), id.Name, c, lc))
				}
			}
		}
	}
	return out
}
