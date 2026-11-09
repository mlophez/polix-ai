package architecture

import (
	"fmt"
	"go/ast"
	"sort"
	"testing"
)

// GOARCH-34: every use case has its two tests. logic_test.go calls the logic Decide, does not mention Port nor
// Adapter and uses no name declared in in_test.go, where the fake of the port lives; in_test.go calls the shell
// run and does not mention Adapter. Port and Adapter count only as unqualified identifiers, so a field x.Port is no
// match. The optional out_test.go is never missing. That the logic is tested with domain values and the shell with
// a fake is left to the review.
func TestGOARCH34(t *testing.T) {
	checkRule(t, checkUseCaseTests,
		"internal/app/order/place/logic_test.go: uses Port: the logic is tested with domain values, without fakes",
		"internal/app/order/place: use case PlaceOrder has no in_test.go",
		"internal/app/stock/count/logic_test.go: does not call the logic Decide",
		"internal/app/stock/count/logic_test.go: uses fakePort, declared in in_test.go: the logic is tested with "+
			"domain values, without fakes",
		"internal/app/stock/count/in_test.go: does not call the shell run",
		"internal/app/stock/count/in_test.go: uses Adapter: the shell is tested with a fake of the port")
}

func checkUseCaseTests(m *module) []string {
	tests := map[string]*goFile{}
	for _, f := range m.files {
		if f.kind == kindTest {
			tests[f.dir+"/"+f.name] = f
		}
	}
	const logicWhy, shellWhy = "the logic is tested with domain values, without fakes",
		"the shell is tested with a fake of the port"
	var out []string
	for _, uc := range m.useCases() {
		for _, want := range []struct {
			name, calls, what string
			banned            []string
			why               string
		}{
			{"logic_test.go", "Decide", "the logic", []string{"Port", "Adapter"}, logicWhy},
			{"in_test.go", "run", "the shell", []string{"Adapter"}, shellWhy},
		} {
			f := tests[uc.dir+"/"+want.name]
			if f == nil {
				out = append(out, fmt.Sprintf("%s: use case %s has no %s", uc.dir, uc.name(), want.name))
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
		logic, in := tests[uc.dir+"/logic_test.go"], tests[uc.dir+"/in_test.go"]
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

// usesName reports whether f mentions the identifier name unqualified: the selector of x.name, a field or a
// method of something, is no match.
func usesName(f *ast.File, name string) bool {
	selected := map[*ast.Ident]bool{}
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			selected[x.Sel] = true
		case *ast.Ident:
			if x.Name == name && !selected[x] {
				found = true
			}
		}
		return !found
	})
	return found
}
