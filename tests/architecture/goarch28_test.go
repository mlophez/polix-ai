package architecture

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
// field of a T (x.F = v, x.F += v, x.F++, x.F[i] = v), reached directly or promoted through embedding, not even
// the methods of another domain struct that holds a T. The tests are left out.
func TestGOARCH28(t *testing.T) {
	checkRule(t, checkChangeThroughMethods,
		"order_line.go: Relabel assigns the field SKU of order.Line outside its methods",
		"cancel_order_in.go: CancelOrderCommand assigns the field SKU of order.Line outside its methods",
		"Note.Fix assigns the field SKU of order.Line outside its methods",
		"Note.Fix assigns the field price of order.Line outside its methods",
		"Pack.Tag assigns the field Tags of order.Line outside its methods",
		"Pack.Tag assigns the field SKU of order.Line outside its methods",
		"Bundle.Mark assigns the field SKU of order.Line outside its methods")
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
			who := fn.Name.Name
			if r := receiver(fn); r != "" {
				who = r + "." + who
			}
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
						if key := m.domainStruct(declaringType(selection)); key != "" && key != owner {
							out = append(out, fmt.Sprintf("%s: %s assigns the field %s of %s outside its methods", f.rel, who,
								sel.Sel.Name, shortType(key)))
						}
					}
				}
				return true
			})
		}
	}
	return out
}

// declaringType returns the type that declares the field or method of a selection: the receiver, or the embedded
// field the selection is promoted through.
func declaringType(sel *types.Selection) types.Type {
	t := sel.Recv()
	index := sel.Index()
	for _, i := range index[:len(index)-1] {
		st, ok := structOf(t)
		if !ok {
			return t
		}
		t = st.Field(i).Type()
	}
	return t
}

// structOf returns the struct t is, through aliases and pointers.
func structOf(t types.Type) (*types.Struct, bool) {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	st, ok := t.Underlying().(*types.Struct)
	return st, ok
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
