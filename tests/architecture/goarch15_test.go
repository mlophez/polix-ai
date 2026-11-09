package architecture

import (
	"fmt"
	"go/ast"
	"go/types"
	"testing"
)

// cobraPath is the import path of cobra, whose *cobra.Command is the entry of a use case.
const cobraPath = "github.com/spf13/cobra"

// GOARCH-15: the only exported function of the in files is the entry Command, in in.go, and a sibling
// in_<topic>.go exports no name: no function, type, constant nor variable (its methods are not names of the
// package). Command returns a *cobra.Command, but in a load use case, whose entry returns the loader of its domain,
// <noun>.Load.
func TestGOARCH15(t *testing.T) {
	checkRule(t, checkOnlyCommandExported,
		"internal/app/order/place/in.go:13: exports Helper: the only exported function is Command",
		"internal/app/stock/count/in_scan.go:10: exports Scan: the only exported function is Command, in in.go, "+
			"and a sibling of in.go exports no name",
		"internal/app/order/place/in.go:9: Command returns nothing: the entry of a use case returns *cobra.Command",
		"internal/app/order/load/in.go:7: Command returns *cobra.Command: the entry of a load use case returns the "+
			"loader of its domain, order.Load")
	checkAccepts(t, checkOnlyCommandExported, "internal/app/stock/take/", "internal/app/stock/load/")
}

func checkOnlyCommandExported(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		if uc.files[kindIn] == nil {
			continue // GOARCH-3
		}
		command := "Command"
		for _, in := range uc.all[kindIn] {
			for _, d := range in.ast.Decls {
				if in.sibling() {
					for _, id := range topLevelNames(d) {
						if id.IsExported() {
							out = append(out, fmt.Sprintf("%s: exports %s: the only exported function is %s, in in.go, "+
								"and a sibling of in.go exports no name", m.pos(in, id), id.Name, command))
						}
					}
					continue
				}
				fn, ok := d.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || !fn.Name.IsExported() {
					continue
				}
				if fn.Name.Name != command {
					out = append(out, fmt.Sprintf("%s: exports %s: the only exported function is %s", m.pos(in, fn),
						fn.Name.Name, command))
					continue
				}
				if v := entryResult(m, uc, in, fn); v != "" {
					out = append(out, fmt.Sprintf("%s: %s", m.pos(in, fn), v))
				}
			}
		}
	}
	return out
}

// entryResult tells what is wrong with the result of the entry fn of uc, declared in in, or "": one
// *cobra.Command, or in a load use case one <noun>.Load of its domain.
func entryResult(m *module, uc *useCase, in *goFile, fn *ast.FuncDecl) string {
	got := "nothing"
	var result ast.Expr
	if res := fn.Type.Results; res != nil && len(res.List) > 0 {
		var all []string
		for _, r := range res.List {
			all = append(all, types.ExprString(r.Type))
		}
		got = fmt.Sprint(all)
		if len(res.List) == 1 && len(res.List[0].Names) <= 1 {
			result, got = res.List[0].Type, types.ExprString(res.List[0].Type)
		}
	}
	if uc.verb == loadVerb {
		if pkg, name := selector(result); name == "Load" && m.moduleDir(imports(in.ast)[pkg]) == "internal/app/"+uc.noun {
			return ""
		}
		return fmt.Sprintf("Command returns %s: the entry of a load use case returns the loader of its domain, %s.Load",
			got, uc.noun)
	}
	if star, ok := result.(*ast.StarExpr); ok {
		if pkg, name := selector(star.X); name == "Command" && imports(in.ast)[pkg] == cobraPath {
			return ""
		}
	}
	return fmt.Sprintf("Command returns %s: the entry of a use case returns *cobra.Command, and only the entry of a "+
		"load use case the loader of its domain", got)
}

// selector returns the package and the name of a qualified identifier pkg.Name, or "".
func selector(e ast.Expr) (string, string) {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}
	id, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", ""
	}
	return id.Name, sel.Sel.Name
}
