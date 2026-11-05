package tests

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
	"testing"
)

// GOARCH-23: every use case has the same shape. Its logic exports only the function <Verb><Noun> and the type
// <Verb><Noun>Output; the function takes domain values (types of the domain of any package of the module, or
// predeclared types, also as slices, maps and pointers) and returns <Verb><Noun>Output or
// (<Verb><Noun>Output, error); and the entry uses of the logic only those two names. A package of internal/ that
// is not in the module cannot be checked and its types are taken as domain.
func TestGOARCH23LogicShape(t *testing.T) {
	checkRule(t, checkLogicShape,
		"exports PlaceOrderInput: the logic exports only PlaceOrder and PlaceOrderOutput",
		"exports PlaceOrderTotal: the logic exports only PlaceOrder and PlaceOrderOutput",
		"PlaceOrder takes PlaceOrderInput, which is not a domain type",
		"PlaceOrder returns (PlaceOrderOutput, int), not PlaceOrderOutput nor (PlaceOrderOutput, error)",
		"place_order_in.go:16: the entry uses PlaceOrderInput of the logic: only PlaceOrder and PlaceOrderOutput")
}

func checkLogicShape(m *module) []string {
	var out []string
	domain := m.domainSymbols()
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
			for _, r := range localRefs(in, syms[in.dir]) {
				if r.decl == logic && !allowed[r.name] {
					out = append(out, fmt.Sprintf("%s: the entry uses %s of the logic: %s", m.pos(in, r.node), r.name, only))
				}
			}
		}
	}
	return out
}

// topLevelNames returns the names a declaration adds to its package; methods add none.
func topLevelNames(d ast.Decl) []*ast.Ident {
	var out []*ast.Ident
	switch d := d.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil {
			out = append(out, d.Name)
		}
	case *ast.GenDecl:
		for _, s := range d.Specs {
			switch s := s.(type) {
			case *ast.TypeSpec:
				out = append(out, s.Name)
			case *ast.ValueSpec:
				out = append(out, s.Names...)
			}
		}
	}
	return out
}

// logicSignature checks that the function of the logic fn takes domain types and returns its output.
func logicSignature(m *module, f *goFile, fn *ast.FuncDecl, domain map[string]map[string]bool) []string {
	var out []string
	imps := imports(f.ast)
	for _, p := range fn.Type.Params.List {
		if !domainType(p.Type, f, domain, imps) {
			out = append(out, fmt.Sprintf("%s: %s takes %s, which is not a domain type", m.pos(f, p), fn.Name.Name,
				types.ExprString(p.Type)))
		}
	}
	output := fn.Name.Name + "Output"
	var results []string
	if fn.Type.Results != nil {
		for _, r := range fn.Type.Results.List {
			for range max(1, len(r.Names)) {
				results = append(results, types.ExprString(r.Type))
			}
		}
	}
	ok := (len(results) == 1 && results[0] == output) ||
		(len(results) == 2 && results[0] == output && results[1] == "error")
	if !ok {
		out = append(out, fmt.Sprintf("%s: %s returns (%s), not %s nor (%s, error)", m.pos(f, fn), fn.Name.Name,
			strings.Join(results, ", "), output, output))
	}
	return out
}

// domainType reports whether e is a predeclared type or a type of the domain of a package of the module, also as
// a slice, an array, a map, a pointer or a variadic parameter of them.
func domainType(e ast.Expr, f *goFile, domain map[string]map[string]bool, imps map[string]string) bool {
	switch t := e.(type) {
	case *ast.Ident:
		if _, ok := types.Universe.Lookup(t.Name).(*types.TypeName); ok {
			return true
		}
		return domain[f.dir][t.Name]
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok {
			return false
		}
		dir := internalDir(imps[id.Name])
		if dir == "" {
			return false
		}
		names, inModule := domain[dir]
		return !inModule || names[t.Sel.Name]
	case *ast.StarExpr:
		return domainType(t.X, f, domain, imps)
	case *ast.ArrayType:
		return domainType(t.Elt, f, domain, imps)
	case *ast.Ellipsis:
		return domainType(t.Elt, f, domain, imps)
	case *ast.MapType:
		return domainType(t.Key, f, domain, imps) && domainType(t.Value, f, domain, imps)
	}
	return false
}
