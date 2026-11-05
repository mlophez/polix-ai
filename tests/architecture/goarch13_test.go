package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"
)

// GOARCH-13: in every file the types come first, right after the imports, before constants, variables and
// functions.
func TestGOARCH13(t *testing.T) {
	checkRule(t, checkTypesFirst, "internal/order/order.go: type Item after constants, variables or functions")
}

func checkTypesFirst(m *module) []string {
	var out []string
	for _, f := range m.files {
		other := false
		for _, d := range f.ast.Decls {
			g, ok := d.(*ast.GenDecl)
			switch {
			case ok && g.Tok == token.IMPORT:
			case ok && g.Tok == token.TYPE:
				if other {
					for _, s := range g.Specs {
						out = append(out, fmt.Sprintf("%s: type %s after constants, variables or functions", f.rel,
							s.(*ast.TypeSpec).Name.Name))
					}
				}
			default:
				other = true
			}
		}
	}
	return out
}
