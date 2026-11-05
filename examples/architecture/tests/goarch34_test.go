package tests

import (
	"fmt"
	"go/ast"
	"sort"
	"testing"
)

// GOARCH-34: every use case has its two tests. <verb>_<noun>_logic_test.go calls the logic <Verb><Noun>, does
// not mention <V>Port nor <V>Adapter and uses no name declared in <verb>_<noun>_in_test.go, where the fake of
// the port lives; <verb>_<noun>_in_test.go calls the shell <verb><Noun> and does not mention <V>Adapter. The
// optional <verb>_<noun>_out_test.go is never missing. That the logic is tested with domain values and the shell
// with a fake is left to the review.
func TestGOARCH34UseCaseTests(t *testing.T) {
	checkRule(t, checkUseCaseTests,
		"internal/order/place_order_logic_test.go: uses PlaceOrderPort: the logic is tested with domain values, "+
			"without fakes",
		"internal/order: use case place_order has no place_order_in_test.go",
		"internal/stock/count_stock_logic_test.go: does not call the logic CountStock",
		"internal/stock/count_stock_logic_test.go: uses fakeCountStockPort, declared in count_stock_in_test.go: "+
			"the logic is tested with domain values, without fakes",
		"internal/stock/count_stock_in_test.go: does not call the shell countStock",
		"internal/stock/count_stock_in_test.go: uses CountStockAdapter: the shell is tested with a fake of the port")
}

func checkUseCaseTests(m *module) []string {
	tests := map[string]*goFile{}
	for _, f := range m.files {
		if f.kind == kindTest {
			tests[f.dir+"/"+f.name] = f
		}
	}
	var out []string
	for _, uc := range m.useCases() {
		c, lc := camel(uc.stem), lowerCamel(uc.stem)
		logicWhy, shellWhy := "the logic is tested with domain values, without fakes",
			"the shell is tested with a fake of the port"
		for _, want := range []struct {
			suffix, calls, what string
			banned              []string
			why                 string
		}{
			{"_logic_test.go", c, "the logic", []string{c + "Port", c + "Adapter"}, logicWhy},
			{"_in_test.go", lc, "the shell", []string{c + "Adapter"}, shellWhy},
		} {
			name := uc.stem + want.suffix
			f := tests[uc.dir+"/"+name]
			if f == nil {
				out = append(out, fmt.Sprintf("%s: use case %s has no %s", uc.dir, uc.stem, name))
				continue
			}
			if !callsName(f.ast, want.calls) {
				out = append(out, fmt.Sprintf("%s: does not call %s %s", f.rel, want.what, want.calls))
			}
			for _, b := range want.banned {
				if usesName(f.ast, b) {
					out = append(out, fmt.Sprintf("%s: uses %s: %s", f.rel, b, want.why))
				}
			}
		}
		logic, in := tests[uc.dir+"/"+uc.stem+"_logic_test.go"], tests[uc.dir+"/"+uc.stem+"_in_test.go"]
		if logic == nil || in == nil {
			continue
		}
		var names []string
		for name := range decls(in.ast) {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if name != "_" && usesName(logic.ast, name) {
				out = append(out, fmt.Sprintf("%s: uses %s, declared in %s: %s", logic.rel, name, in.name, logicWhy))
			}
		}
	}
	return out
}

// callsName reports whether f calls the function name of its package.
func callsName(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
				found = true
			}
		}
		return !found
	})
	return found
}

// usesName reports whether f mentions the identifier name outside its own declarations.
func usesName(f *ast.File, name string) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == name {
			found = true
		}
		return !found
	})
	return found
}
