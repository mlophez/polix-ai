package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"regexp"
	"testing"
)

// constructor matches the names of constructors.
var constructor = regexp.MustCompile(`^[Nn]ew([A-Z]|$)`)

// GOARCH-29: there are no constructors (New*, new*), and a value of a domain struct built with a literal with
// fields (T{...} or &T{...}), in a function or at the top level of a file, is bound to a variable on which the
// same function calls Valid() afterwards and uses its error: neither alone as a statement, deferred, in a
// goroutine nor assigned to _. The empty literal T{}, the zero value returned with an error, and the tests are
// left out.
func TestGOARCH29(t *testing.T) {
	checkRule(t, checkBuildThenValid,
		"internal/app/order/order_line.go: builds a order.Line in other but never calls other.Valid()",
		"order_line.go: builds a order.Line that is not bound to a variable, so it is never validated",
		"order_line.go: builds a order.Line that is assigned to _, so it is never validated",
		"builds a order.Line in l but discards the error of l.Valid()",
		"builds a order.Line in p but discards the error of p.Valid()",
		"builds a order.Line in d but discards the error of d.Valid()",
		"builds a order.Line in g but discards the error of g.Valid()",
		"internal/app/order/cancel/in.go:38: builds a order.Line at the top level of the file, where no function "+
			"validates it",
		"internal/app/order/place/out.go:7: declares the constructor NewAdapter",
		"declares the constructor newLine")
}

func checkBuildThenValid(m *module) []string {
	info := m.typed()
	var out []string
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && constructor.MatchString(fn.Name.Name) {
				out = append(out, fmt.Sprintf("%s: declares the constructor %s: build values with a literal and Valid",
					m.pos(f, fn), fn.Name.Name))
			}
		}
	}
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body != nil {
					out = append(out, literalsIn(m, info, f, d.Body)...)
				}
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					out = append(out, topLevelLiterals(m, info, f, d)...)
				}
			}
		}
	}
	return out
}

// literalsIn returns the literals of domain structs in body that are not validated afterwards.
func literalsIn(m *module, info *types.Info, f *goFile, body *ast.BlockStmt) []string {
	var out []string
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if lit, ok := n.(*ast.CompositeLit); ok && len(lit.Elts) > 0 {
			if key := m.domainStruct(info.TypeOf(lit)); key != "" {
				if v := literalProblem(info, body, lit, stack); v != "" {
					out = append(out, fmt.Sprintf("%s: builds a %s %s", f.rel, shortType(key), v))
				}
			}
		}
		stack = append(stack, n)
		return true
	})
	return out
}

// topLevelLiterals returns the literals of domain structs in the values of a package variable: nothing validates
// them.
func topLevelLiterals(m *module, info *types.Info, f *goFile, d *ast.GenDecl) []string {
	var out []string
	for _, s := range d.Specs {
		for _, v := range s.(*ast.ValueSpec).Values {
			ast.Inspect(v, func(n ast.Node) bool {
				if lit, ok := n.(*ast.CompositeLit); ok && len(lit.Elts) > 0 {
					if key := m.domainStruct(info.TypeOf(lit)); key != "" {
						out = append(out, fmt.Sprintf("%s: builds a %s at the top level of the file, where no function "+
							"validates it", m.pos(f, lit), shortType(key)))
					}
				}
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
	switch {
	case id == nil:
		return "that is not bound to a variable, so it is never validated"
	case id.Name == "_":
		return "that is assigned to _, so it is never validated"
	}
	obj := info.ObjectOf(id)
	called, used := false, false
	var parents []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			parents = parents[:len(parents)-1]
			return true
		}
		if call, ok := n.(*ast.CallExpr); ok && call.Pos() > lit.End() && validCall(info, call, id.Name, obj) {
			called = true
			if !discarded(call, parents[len(parents)-1]) {
				used = true
			}
		}
		parents = append(parents, n)
		return true
	})
	switch {
	case used:
		return ""
	case called:
		return fmt.Sprintf("in %s but discards the error of %s.Valid()", id.Name, id.Name)
	}
	return fmt.Sprintf("in %s but never calls %s.Valid()", id.Name, id.Name)
}

// validCall reports whether call is name.Valid(), where name is the variable obj.
func validCall(info *types.Info, call *ast.CallExpr, name string, obj types.Object) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Valid" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == name && (obj == nil || info.ObjectOf(x) == obj)
}

// discarded reports whether the result of call is thrown away: call alone as a statement, deferred, in a
// goroutine, or assigned to _.
func discarded(call *ast.CallExpr, parent ast.Node) bool {
	switch p := parent.(type) {
	case *ast.ExprStmt, *ast.DeferStmt, *ast.GoStmt:
		return true
	case *ast.AssignStmt:
		for i, r := range p.Rhs {
			if r == call && i < len(p.Lhs) {
				id, ok := p.Lhs[i].(*ast.Ident)
				return ok && id.Name == "_"
			}
		}
	case *ast.ValueSpec:
		for i, v := range p.Values {
			if v == call && i < len(p.Names) {
				return p.Names[i].Name == "_"
			}
		}
	}
	return false
}
