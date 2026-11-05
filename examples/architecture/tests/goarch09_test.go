package tests

import (
	"fmt"
	"testing"
)

// GOARCH-9: the domain uses only domain: the domain files of its own package and, of another package of internal/,
// only the names its domain files declare; pkg/ and the packages outside the module are free (GOARCH-4 limits
// them). A package of internal/ that is not in the module cannot be checked and is left out.
func TestGOARCH09DomainUsesDomain(t *testing.T) {
	checkRule(t, checkDomainUsesDomain, "the domain uses placeOrderLimit, of place_order_logic.go",
		"uses stock.CountStock, which is not domain of internal/stock")
}

func checkDomainUsesDomain(m *module) []string {
	var out []string
	domain := m.domainSymbols()
	for _, files := range m.packages() {
		syms := symbols(files)
		for _, f := range files {
			if f.kind != kindDomain {
				continue
			}
			for _, r := range m.localRefs(f, syms) {
				if r.decl.kind != kindDomain {
					out = append(out, fmt.Sprintf("%s: the domain uses %s, of %s", m.pos(f, r.node), r.name, r.decl.name))
				}
			}
			out = append(out, foreignNonDomain(m, f, domain)...)
		}
	}
	return out
}

// foreignNonDomain returns the uses in f of names of other packages of the module that are not their domain.
func foreignNonDomain(m *module, f *goFile, domain map[string]map[string]bool) []string {
	var out []string
	for _, r := range m.packageRefs(f) {
		if names, ok := domain[r.dir]; ok && !names[r.name] {
			out = append(out, fmt.Sprintf("%s: uses %s.%s, which is not domain of %s", m.pos(f, r.node), r.pkg, r.name, r.dir))
		}
	}
	return out
}
