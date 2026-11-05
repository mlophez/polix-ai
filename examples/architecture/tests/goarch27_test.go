package tests

import (
	"fmt"
	"go/ast"
	"regexp"
	"testing"
)

// setGet matches the names of setters and getters.
var setGet = regexp.MustCompile(`^(Set|Get)([A-Z]|$)`)

// GOARCH-27: the methods of a domain struct have a value receiver, so a method that changes the value returns a
// copy and the value is immutable; and they are no setters nor getters: no Set* nor Get*, no method that only
// returns a field, no method that only copies its parameters into fields.
func TestGOARCH27DomainMethods(t *testing.T) {
	checkRule(t, checkDomainMethods,
		"Line.SetSKU is named as a setter or a getter",
		"Line.Rename is a setter: it only copies its parameters into fields",
		"Line.Code is a getter: it only returns a field",
		"Line.Discount has a pointer receiver: a method that changes the value returns a copy")
}

func checkDomainMethods(m *module) []string {
	structs := m.domainStructs()
	var out []string
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || !structs[f.dir+"."+receiver(fn)] {
				continue
			}
			name := receiver(fn) + "." + fn.Name.Name
			if _, ptr := fn.Recv.List[0].Type.(*ast.StarExpr); ptr {
				out = append(out, fmt.Sprintf("%s: %s has a pointer receiver: a method that changes the value returns "+
					"a copy", m.pos(f, fn), name))
			}
			if setGet.MatchString(fn.Name.Name) {
				out = append(out, fmt.Sprintf("%s: %s is named as a setter or a getter", m.pos(f, fn), name))
			}
			recv := ""
			if len(fn.Recv.List[0].Names) > 0 {
				recv = fn.Recv.List[0].Names[0].Name
			}
			if recv != "" && fn.Body != nil && isGetter(fn, recv) {
				out = append(out, fmt.Sprintf("%s: %s is a getter: it only returns a field", m.pos(f, fn), name))
			}
			if recv != "" && fn.Body != nil && isSetter(fn, recv) {
				out = append(out, fmt.Sprintf("%s: %s is a setter: it only copies its parameters into fields",
					m.pos(f, fn), name))
			}
		}
	}
	return out
}

// isGetter reports whether fn takes nothing and only returns a field of its receiver recv.
func isGetter(fn *ast.FuncDecl, recv string) bool {
	if len(fn.Type.Params.List) > 0 || len(fn.Body.List) != 1 {
		return false
	}
	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	return ok && len(ret.Results) == 1 && onReceiver(ret.Results[0], recv)
}

// isSetter reports whether fn only assigns its parameters to fields of its receiver recv, then returns.
func isSetter(fn *ast.FuncDecl, recv string) bool {
	stmts := fn.Body.List
	if len(stmts) < 2 {
		return false
	}
	if _, ok := stmts[len(stmts)-1].(*ast.ReturnStmt); !ok {
		return false
	}
	params := map[string]bool{}
	for _, p := range fn.Type.Params.List {
		for _, n := range p.Names {
			params[n.Name] = true
		}
	}
	for _, s := range stmts[:len(stmts)-1] {
		a, ok := s.(*ast.AssignStmt)
		if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 || !onReceiver(a.Lhs[0], recv) {
			return false
		}
		if id, ok := a.Rhs[0].(*ast.Ident); !ok || !params[id.Name] {
			return false
		}
	}
	return true
}

// onReceiver reports whether e is a field of the receiver recv: recv.Field.
func onReceiver(e ast.Expr, recv string) bool {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == recv
}
