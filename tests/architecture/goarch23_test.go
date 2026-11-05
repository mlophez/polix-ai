package architecture

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
	"testing"
)

// useCaseSuffixNames end the names of the types of a use case: never a domain type, whatever their package.
var useCaseSuffixNames = []string{"Output", "Request", "Response", "Port", "Adapter"}

// GOARCH-23: every use case has the same shape. Its logic exports only the function <Verb><Noun> and the type
// <Verb><Noun>Output; the function takes domain values (types of the domain of any package of the module, or
// predeclared types, also as slices, maps and pointers) and returns <Verb><Noun>Output and nothing else; and the
// in file uses of the logic only those two names. A package of internal/ that is not in the module cannot be
// checked and its types are taken as domain, but those named *Output, *Request, *Response, *Port or *Adapter.
func TestGOARCH23(t *testing.T) {
	checkRule(t, checkLogicShape,
		"exports PlaceOrderInput: the logic exports only PlaceOrder and PlaceOrderOutput",
		"exports PlaceOrderTotal: the logic exports only PlaceOrder and PlaceOrderOutput",
		"PlaceOrder takes PlaceOrderInput, which is not a domain type",
		"PlaceOrder returns (PlaceOrderOutput, error), not PlaceOrderOutput: a refusal is data of the output",
		"CountStock returns (int), not CountStockOutput",
		"CountStock takes StockPort, which is not a domain type",
		"place_order_in.go: the entry uses PlaceOrderInput of the logic: only PlaceOrder and PlaceOrderOutput")
}

func checkLogicShape(m *module) []string {
	var out []string
	domain := m.domainTypes()
	syms := map[string]map[string]*goFile{}
	for dir, files := range m.packages() {
		syms[dir] = symbols(files)
	}
	for _, uc := range m.useCases() {
		c := camel(uc.stem)
		allowed := map[string]bool{c: true, c + "Output": true}
		only := fmt.Sprintf("only %s and %sOutput", c, c)
		logic := uc.files[kindLogic]
		if logic == nil {
			continue // GOARCH-3
		}
		for _, d := range logic.ast.Decls {
			for _, id := range topLevelNames(d) {
				if id.IsExported() && !allowed[id.Name] {
					out = append(out, fmt.Sprintf("%s: exports %s: the logic exports %s", m.pos(logic, id), id.Name, only))
				}
			}
		}
		if fn := funcDecl(logic.ast, c); fn != nil {
			out = append(out, logicSignature(m, logic, fn, domain)...)
		}
		if in := uc.files[kindIn]; in != nil {
			for _, r := range m.localRefs(in, syms[in.dir]) {
				if r.decl == logic && !allowed[r.name] {
					out = append(out, fmt.Sprintf("%s: the entry uses %s of the logic: %s", in.rel, r.name, only))
				}
			}
		}
	}
	return out
}

// logicSignature checks that the function of the logic fn takes domain types and returns only its output.
func logicSignature(m *module, f *goFile, fn *ast.FuncDecl, domain map[string]map[string]bool) []string {
	var out []string
	imps := imports(f.ast)
	for _, p := range fn.Type.Params.List {
		if !m.domainType(p.Type, f, domain, imps) {
			out = append(out, fmt.Sprintf("%s: %s takes %s, which is not a domain type", m.pos(f, p), fn.Name.Name,
				types.ExprString(p.Type)))
		}
	}
	output := fn.Name.Name + "Output"
	results := fieldTypes(fn.Type.Results)
	if len(results) != 1 || results[0] != output {
		out = append(out, fmt.Sprintf("%s: %s returns (%s), not %s: a refusal is data of the output, never an error",
			m.pos(f, fn), fn.Name.Name, strings.Join(results, ", "), output))
	}
	return out
}

// domainType reports whether e is a predeclared type or a type of the domain of a package of the module, also as
// a slice, an array, a map, a pointer or a variadic parameter of them. A name with the suffix of a type of a use
// case never is.
func (m *module) domainType(e ast.Expr, f *goFile, domain map[string]map[string]bool, imps map[string]string) bool {
	switch t := e.(type) {
	case *ast.Ident:
		if _, ok := types.Universe.Lookup(t.Name).(*types.TypeName); ok {
			return true
		}
		return !useCaseSuffix(t.Name) && domain[f.dir][t.Name]
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok || useCaseSuffix(t.Sel.Name) {
			return false
		}
		dir := m.internalDir(imps[id.Name])
		if dir == "" {
			return false
		}
		names, inModule := domain[dir]
		return !inModule || names[t.Sel.Name]
	case *ast.StarExpr:
		return m.domainType(t.X, f, domain, imps)
	case *ast.ArrayType:
		return m.domainType(t.Elt, f, domain, imps)
	case *ast.Ellipsis:
		return m.domainType(t.Elt, f, domain, imps)
	case *ast.MapType:
		return m.domainType(t.Key, f, domain, imps) && m.domainType(t.Value, f, domain, imps)
	}
	return false
}

// useCaseSuffix reports whether name ends as a type of a use case: Output, Request, Response, Port or Adapter.
func useCaseSuffix(name string) bool {
	for _, s := range useCaseSuffixNames {
		if strings.HasSuffix(name, s) && name != s {
			return true
		}
	}
	return false
}
