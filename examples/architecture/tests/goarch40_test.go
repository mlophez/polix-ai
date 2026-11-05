package tests

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-40: the methods of a domain struct are declared in the domain files of its package, never in the files
// of a use case: no use case adds behavior to the domain that another use case could reach.
func TestGOARCH40DomainMethodsInDomainFiles(t *testing.T) {
	checkRule(t, checkDomainMethodsInDomainFiles,
		"cancel_order_in.go: declares the method Line.CancelOrderLabel of the domain struct Line "+
			"in a file of a use case")
}

func checkDomainMethodsInDomainFiles(m *module) []string {
	structs := m.domainStructs()
	var out []string
	for _, f := range m.sources() {
		if f.kind == kindDomain {
			continue
		}
		for _, d := range f.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			if r := receiver(fn); structs[f.dir+"."+r] {
				out = append(out, fmt.Sprintf("%s: declares the method %s.%s of the domain struct %s in a file of a "+
					"use case: the domain is in %s.go and %s_*.go", f.rel, r, fn.Name.Name, r, f.pkg, f.pkg))
			}
		}
	}
	return out
}
