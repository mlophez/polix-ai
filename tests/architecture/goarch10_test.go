package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-10: use cases are vertical: the files of a use case use only the files of the same use case, the domain
// of their own package, the domain of other packages of internal/, pkg/ and the packages outside the module.
func TestGOARCH10(t *testing.T) {
	checkRule(t, checkVertical, "use case cancel_order uses PlaceOrderAdapter, of place_order_out.go",
		"uses stock.CountStock, which is not domain of internal/stock")
}

func checkVertical(m *module) []string {
	var out []string
	domain := m.domainSymbols()
	for _, files := range m.packages() {
		syms := symbols(files)
		for _, f := range files {
			if f.useCase == "" {
				continue
			}
			for _, r := range m.localRefs(f, syms) {
				if r.decl.kind != kindDomain && r.decl.useCase != f.useCase {
					out = append(out, fmt.Sprintf("%s: use case %s uses %s, of %s", m.pos(f, r.node), f.useCase, r.name, r.decl.name))
				}
			}
			out = append(out, foreignNonDomain(m, f, domain)...)
		}
	}
	return out
}
