package tests

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"path"
	"strings"
	"testing"
)

// GOARCH-28: a domain struct changes only through its own methods: no code outside the methods of T assigns to a
// field of a T (x.F = v, x.F += v, x.F++, x.F[i] = v), not even the methods of another domain struct that holds a T.
func TestGOARCH28ChangeThroughMethods(t *testing.T) {
	checkRule(t, checkChangeThroughMethods,
		"internal/order/order_line.go:36: assigns the field SKU of order.Line outside its methods",
		"internal/order/cancel_order_in.go:21: assigns the field SKU of order.Line outside its methods")
}

func checkChangeThroughMethods(m *module) []string {
	info := m.typed()
	var out []string
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			owner := m.receiverOf(f, fn)
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				var targets []ast.Expr
				switch s := n.(type) {
				case *ast.AssignStmt:
					if s.Tok != token.DEFINE {
						targets = s.Lhs
					}
				case *ast.IncDecStmt:
					targets = []ast.Expr{s.X}
				}
				for _, t := range targets {
					for _, sel := range fieldPath(t) {
						selection := info.Selections[sel]
						if selection == nil || selection.Kind() != types.FieldVal {
							continue
						}
						if key := m.domainStruct(selection.Recv()); key != "" && key != owner {
							out = append(out, fmt.Sprintf("%s: assigns the field %s of %s outside its methods",
								m.pos(f, sel), sel.Sel.Name, shortType(key)))
						}
					}
				}
				return true
			})
		}
	}
	return out
}

// fieldPath returns the selectors along the target of an assignment: x.A.B[i] gives x.A.B and x.A.
func fieldPath(e ast.Expr) []*ast.SelectorExpr {
	var out []*ast.SelectorExpr
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.SelectorExpr:
			out = append(out, x)
			e = x.X
		default:
			return out
		}
	}
}

// shortType renders folder.Name (internal/order.Line) as order.Line.
func shortType(key string) string {
	dir, name, _ := strings.Cut(key, ".")
	return path.Base(dir) + "." + name
}
