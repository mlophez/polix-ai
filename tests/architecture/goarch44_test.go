package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

// whereFunctionsGo says where a function that is not a method of a domain goes (GOARCH-44).
const whereFunctionsGo = "a translation of a format goes to the in or out files of a use case or to a load use " +
	"case, shared logic to a method of a domain type, the logic of one use case to its logic files"

// GOARCH-44: a domain package (internal/app/<noun>/*.go, tests aside) declares only types, interfaces, constants,
// variables that are not functions, and methods of its types, structs or named types: no function that is not a
// method, no init, and no package variable whose value is a function (a function literal, or a value of a function
// type by the checker).
func TestGOARCH44(t *testing.T) {
	checkRule(t, checkDomainFunctions,
		"internal/app/order/order.go:11: declares the function Total, which is not a method: "+whereFunctionsGo,
		"internal/app/order/order_line.go:70: declares the function newLine, which is not a method",
		"internal/app/order/order_events.go:4: declares init, a function that is not a method",
		"internal/app/order/order_events.go:7: declares the variable onPlace, whose value is a function")
	checkAccepts(t, checkDomainFunctions, "internal/app/order/order_line.go:17:",
		"internal/app/stock/stock_units.go", "internal/app/stock/stock_units_test.go")
}

func checkDomainFunctions(m *module) []string {
	var out []string
	for _, f := range m.sources() {
		if f.place.level != levelDomain {
			continue
		}
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				out = append(out, m.freeFunction(f, d)...)
			case *ast.GenDecl:
				if d.Tok == token.VAR {
					out = append(out, m.functionVars(f, d)...)
				}
			}
		}
	}
	return out
}

// freeFunction reports fn when it is not a method.
func (m *module) freeFunction(f *goFile, fn *ast.FuncDecl) []string {
	switch {
	case fn.Recv != nil:
		return nil
	case fn.Name.Name == "init":
		return []string{fmt.Sprintf("%s: declares init, a function that is not a method: a domain runs nothing "+
			"when its package loads", m.pos(f, fn))}
	}
	return []string{fmt.Sprintf("%s: declares the function %s, which is not a method: %s", m.pos(f, fn),
		fn.Name.Name, whereFunctionsGo)}
}

// functionVars reports the variables of the declaration g whose value is a function: a function literal, or a value
// of a function type by the checker.
func (m *module) functionVars(f *goFile, g *ast.GenDecl) []string {
	var out []string
	for _, spec := range valueSpecs(g) {
		for i, id := range spec.Names {
			var value ast.Expr
			if i < len(spec.Values) {
				value = spec.Values[i]
			}
			if _, lit := value.(*ast.FuncLit); lit || m.isFunction(id) {
				out = append(out, fmt.Sprintf("%s: declares the variable %s, whose value is a function: %s",
					m.pos(f, id), id.Name, whereFunctionsGo))
			}
		}
	}
	return out
}

// isFunction reports whether the checker gives the variable id a function type.
func (m *module) isFunction(id *ast.Ident) bool {
	obj := m.typed().Defs[id]
	if obj == nil {
		return false
	}
	_, isSig := obj.Type().Underlying().(*types.Signature)
	return isSig
}
