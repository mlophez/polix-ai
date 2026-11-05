package tests

import (
	"fmt"
	"path"
	"strings"
	"testing"
)

// GOARCH-11: a use case is named <Verb><Noun>, with the noun of its package, and its files declare the names of
// the use case: the logic <Verb><Noun> and <Verb><Noun>Output; the entry
// <Verb><Noun>Command, <Verb><Noun>Request, <Verb><Noun>Response and <Verb><Noun>Port; the adapter
// <Verb><Noun>Adapter.
func TestGOARCH11Names(t *testing.T) {
	checkRule(t, checkNames, "internal/order/place_order_in.go: declares no type PlaceOrderRequest",
		"internal/order: use case ship_parcel is not named <verb>_order")
}

func checkNames(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		pkg := path.Base(uc.dir)
		if verb, ok := strings.CutSuffix(uc.stem, "_"+pkg); !ok || verb == "" {
			out = append(out, fmt.Sprintf("%s: use case %s is not named <verb>_%s", uc.dir, uc.stem, pkg))
		}
		c := camel(uc.stem)
		want := map[kind][][2]string{
			kindLogic: {{"func", c}, {"type", c + "Output"}},
			kindIn:    {{"func", c + "Command"}, {"type", c + "Request"}, {"type", c + "Response"}, {"type", c + "Port"}},
			kindOut:   {{"type", c + "Adapter"}},
		}
		for k, names := range want {
			f := uc.files[k]
			if f == nil {
				continue // GOARCH-3
			}
			have := decls(f.ast)
			for _, w := range names {
				if have[w[1]] != w[0] {
					out = append(out, fmt.Sprintf("%s: declares no %s %s", f.rel, w[0], w[1]))
				}
			}
		}
	}
	return out
}
