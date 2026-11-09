package architecture

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"
	"testing"
)

// reservedNames are the fixed names of a use case: a domain or a helper never declares them.
var reservedNames = []string{"Command", "Request", "Response", "Port", "Adapter", "Decide", "Output"}

// reservedSuffixes end the names of the types of a use case: never declared by a domain nor a helper, and never a
// domain type, whatever their package.
var reservedSuffixes = []string{"Output", "Request", "Response", "Port", "Adapter"}

// GOARCH-23: every use case has the same shape. Its logic exports only the function Decide and the type Output,
// in logic.go, and a sibling logic_<topic>.go exports no name; Decide takes domain values (types of the domain of
// any noun, or predeclared types, also as slices, maps and pointers) and returns Output and nothing else; and the
// in files use of the logic files only those two names. The names
// reserved to the use cases (Command, Request, Response, Port, Adapter, Decide, Output, and any *Output,
// *Request, *Response, *Port or *Adapter) are never declared by a domain nor a helper, and are never a domain
// type. A domain package that is not in the module cannot be checked and its types are taken as domain.
func TestGOARCH23(t *testing.T) {
	checkRule(t, checkLogicShape,
		"exports Input: the logic exports only Decide and Output",
		"exports Total: the logic exports only Decide and Output",
		"Decide takes Input, which is not a domain type",
		"Decide returns (Output, error), not Output: a refusal is data of the output",
		"internal/app/stock/count/logic.go:7: Decide returns (int), not Output",
		"Decide takes stock.StockPort, which is not a domain type",
		"place/in.go: the entry uses Input of the logic: only Decide and Output",
		"internal/app/stock/stock.go:18: declares StockPort, a name reserved to the use cases",
		"internal/app/order/place/pricing/pricing.go:12: declares Output, a name reserved to the use cases",
		"internal/app/stock/count/logic_audit.go:6: exports Threshold: the logic exports only Decide and Output, in "+
			"logic.go, and a sibling of logic.go exports no name")
	checkAccepts(t, checkLogicShape, "internal/app/stock/take/")
}

func checkLogicShape(m *module) []string {
	var out []string
	domain := m.domainTypes()
	allowed := map[string]bool{"Decide": true, "Output": true}
	const only = "only Decide and Output"
	for _, uc := range m.useCases() {
		logic := uc.files[kindLogic]
		if logic == nil {
			continue // GOARCH-3
		}
		for _, f := range uc.all[kindLogic] {
			for _, d := range f.ast.Decls {
				for _, id := range topLevelNames(d) {
					switch {
					case !id.IsExported():
					case f.sibling():
						out = append(out, fmt.Sprintf("%s: exports %s: the logic exports %s, in logic.go, and a sibling "+
							"of logic.go exports no name", m.pos(f, id), id.Name, only))
					case !allowed[id.Name]:
						out = append(out, fmt.Sprintf("%s: exports %s: the logic exports %s", m.pos(f, id), id.Name, only))
					}
				}
			}
		}
		if fn := funcDecl(logic.ast, "Decide"); fn != nil {
			out = append(out, logicSignature(m, logic, fn, domain)...)
		}
		syms := symbols(append(append([]*goFile{}, uc.all[kindIn]...), uc.all[kindLogic]...))
		for _, in := range uc.all[kindIn] {
			for _, r := range m.localRefs(in, syms) {
				if r.decl.kind == kindLogic && !allowed[r.name] {
					out = append(out, fmt.Sprintf("%s: the entry uses %s of the logic: %s", in.rel, r.name, only))
				}
			}
		}
	}
	for _, f := range m.sources() {
		if f.kind != kindDomain && f.kind != kindHelper {
			continue
		}
		for _, d := range f.ast.Decls {
			for _, id := range topLevelNames(d) {
				if reserved(id.Name) {
					out = append(out, fmt.Sprintf("%s: declares %s, a name reserved to the use cases", m.pos(f, id),
						id.Name))
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
	const output = "Output"
	results := fieldTypes(fn.Type.Results)
	if len(results) != 1 || results[0] != output {
		out = append(out, fmt.Sprintf("%s: %s returns (%s), not %s: a refusal is data of the output, never an error",
			m.pos(f, fn), fn.Name.Name, strings.Join(results, ", "), output))
	}
	return out
}

// domainType reports whether e is a predeclared type or a type of a domain package, also as a slice, an array, a
// map, a pointer or a variadic parameter of them. A type of a use case, a helper or a library never is, nor a name
// reserved to the use cases; a domain package that is not in the module is taken as domain.
func (m *module) domainType(e ast.Expr, f *goFile, domain map[string]map[string]bool, imps map[string]string) bool {
	switch t := e.(type) {
	case *ast.Ident:
		if _, ok := types.Universe.Lookup(t.Name).(*types.TypeName); ok {
			return true
		}
		return !reserved(t.Name) && domain[f.dir][t.Name]
	case *ast.SelectorExpr:
		id, ok := t.X.(*ast.Ident)
		if !ok || reserved(t.Sel.Name) {
			return false
		}
		dir := m.domainDir(imps[id.Name])
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

// reserved reports whether name is reserved to the use cases: a fixed name of a use case, or one that ends as the
// type of a use case.
func reserved(name string) bool {
	if contains(reservedNames, name) {
		return true
	}
	for _, s := range reservedSuffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}
