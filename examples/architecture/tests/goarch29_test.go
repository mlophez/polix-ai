package tests

import (
	"fmt"
	"go/ast"
	"go/types"
	"testing"
)

// GOARCH-29: a value of a domain struct built with a literal with fields (T{...} or &T{...}) is bound to a
// variable on which the same function calls Valid() afterwards. The empty literal T{}, the zero value returned
// with an error, and the tests are left out.
func TestGOARCH29BuildThenValid(t *testing.T) {
	checkRule(t, checkBuildThenValid,
		"internal/order/order_line.go:35: builds a order.Line in other but never calls other.Valid()",
		"internal/order/order_line.go:46: builds a order.Line that is not bound to a variable, so it is never validated")
}

func checkBuildThenValid(m *module) []string {
	info := m.typed()
	var out []string
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			var stack []ast.Node
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if n == nil {
					stack = stack[:len(stack)-1]
					return true
				}
				if lit, ok := n.(*ast.CompositeLit); ok && len(lit.Elts) > 0 {
					if key := m.domainStruct(info.TypeOf(lit)); key != "" {
						if v := literalProblem(info, fn.Body, lit, stack); v != "" {
							out = append(out, fmt.Sprintf("%s: builds a %s %s", m.pos(f, lit), shortType(key), v))
						}
					}
				}
				stack = append(stack, n)
				return true
			})
		}
	}
	return out
}

// literalProblem returns why lit, with its ancestors in stack, is not validated in body, or "".
func literalProblem(info *types.Info, body *ast.BlockStmt, lit *ast.CompositeLit, stack []ast.Node) string {
	var node ast.Expr = lit
	i := len(stack) - 1
	if u, ok := stack[i].(*ast.UnaryExpr); ok && i > 0 {
		node, i = u, i-1
	}
	var id *ast.Ident
	switch p := stack[i].(type) {
	case *ast.AssignStmt:
		for k, r := range p.Rhs {
			if r == node && len(p.Lhs) == len(p.Rhs) {
				id, _ = p.Lhs[k].(*ast.Ident)
			}
		}
	case *ast.ValueSpec:
		for k, v := range p.Values {
			if v == node && k < len(p.Names) {
				id = p.Names[k]
			}
		}
	}
	if id == nil || id.Name == "_" {
		return "that is not bound to a variable, so it is never validated"
	}
	obj := info.ObjectOf(id)
	validated := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || call.Pos() < lit.End() {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Valid" {
			return true
		}
		if x, ok := sel.X.(*ast.Ident); ok && x.Name == id.Name && (obj == nil || info.ObjectOf(x) == obj) {
			validated = true
		}
		return true
	})
	if validated {
		return ""
	}
	return fmt.Sprintf("in %s but never calls %s.Valid()", id.Name, id.Name)
}
