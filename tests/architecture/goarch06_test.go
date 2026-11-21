package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"slices"
	"sort"
	"testing"
)

// GOARCH-6: cmd/ only wires, so its files declare nothing but func main, and it has no tests: the commands are
// tested through their entries in internal/. main builds each struct of an adapters package once, only as &pkg.T{}, and
// passes its method Load to the Command of every use case that reads its file.
func TestGOARCH6(t *testing.T) {
	checkRule(t, checkCmdOnlyWires, "cmd/app/main.go: declares helper",
		"cmd/app/main_test.go: cmd has no tests",
		"cmd/app: builds the adapters struct internal/app/stock/adapters.Log 2 times",
		"cmd/vars/main.go:7:", "cmd/vars/main.go:8:",
		"cmd/forms/main.go:8: mentions the adapters struct internal/app/stock/adapters.Log other than as &stockadapters.Log{}",
		"cmd/forms/main.go:9:", "cmd/forms/main.go:10:", "cmd/forms/main.go:11:", "cmd/forms/main.go:12:",
		"cmd/loop/main.go:9: builds the adapters struct internal/app/stock/adapters.Log where it can run more than once",
		"cmd/closure/main.go:7: builds the adapters struct internal/app/stock/adapters.Log where it can run more than once",
		"cmd/gotoloop/main.go:", "uses goto", "cmd/recurse/main.go:", "calls main",
		"cmd/deferloop/main.go:8:")
	checkAccepts(t, checkCmdOnlyWires, "cmd/once/")
}

func checkCmdOnlyWires(m *module) []string {
	var out []string
	builds := map[string]map[string]int{} // by folder of cmd/, the builds of each struct of an adapters package
	for _, f := range m.files {
		if f.place.level != levelCmd {
			continue
		}
		if f.kind == kindTest {
			out = append(out, fmt.Sprintf("%s: cmd has no tests: the commands are tested through their entries in "+
				"internal/", f.rel))
			continue
		}
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Name.Name != "main" || d.Recv != nil {
					out = append(out, fmt.Sprintf("%s: declares %s: cmd only wires the commands in main", f.rel, d.Name.Name))
				}
			case *ast.GenDecl:
				if d.Tok.String() != "import" {
					out = append(out, fmt.Sprintf("%s: declares a %s: cmd only wires the commands in main", m.pos(f, d), d.Tok))
				}
			}
		}
		if builds[f.dir] == nil {
			builds[f.dir] = map[string]int{}
		}
		out = append(out, goesBack(m, f)...)
		counted, notAdmitted := adaptersBuilds(m, f)
		out = append(out, notAdmitted...)
		for name, n := range counted {
			builds[f.dir][name] += n
		}
	}
	for cmd, byStruct := range builds {
		for name, n := range byStruct {
			if n > 1 {
				out = append(out, fmt.Sprintf("%s: builds the adapters struct %s %d times: main builds each adapters struct once "+
					"and passes its method Load to the Command of every use case that reads its file", cmd, name, n))
			}
		}
	}
	sort.Strings(out)
	return out
}

// adaptersBuilds counts the values of f of each struct of an adapters package it imports, by the folder of the package
// and the name of the struct (internal/app/<noun>/adapters.Name). The one form admitted is the literal with its address,
// &pkg.T{}; any other mention of the type (new, var, slice, array, make, a literal without &) is returned in
// notAdmitted, since it builds values the count cannot follow. The literal runs once only in a statement that hangs
// directly from the body of main (see runsOnce). A copy of the value (c := *x) is left to go vet: its copylocks
// analyzer catches it for the structs that hold a lock, and it is harmless for the others.
func adaptersBuilds(m *module, f *goFile) (builds map[string]int, notAdmitted []string) {
	packages := map[string]string{} // local name of the import -> folder of the adapters package
	for name, p := range imports(f.ast) {
		if dir := m.moduleDir(p); placeOf(dir).level == levelAdapters {
			packages[name] = dir
		}
	}
	adaptersType := func(e ast.Expr) (*ast.SelectorExpr, string) {
		if sel, ok := e.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && packages[id.Name] != "" {
				return sel, packages[id.Name] + "." + sel.Sel.Name
			}
		}
		return nil, ""
	}
	admitted := map[*ast.SelectorExpr]bool{}
	repeated := map[*ast.SelectorExpr]bool{} // &pkg.T{} where it can run more than once
	var stack []ast.Node
	ast.Inspect(f.ast, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if sel := addressedAdapters(n, adaptersType); sel != nil {
			if runsOnce(stack) {
				admitted[sel] = true
			} else {
				repeated[sel] = true
			}
		}
		stack = append(stack, n)
		return true
	})
	builds = map[string]int{}
	ast.Inspect(f.ast, func(n ast.Node) bool {
		e, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		sel, name := adaptersType(e)
		switch {
		case sel == nil:
		case admitted[sel]:
			builds[name]++
		case repeated[sel]:
			notAdmitted = append(notAdmitted, fmt.Sprintf("%s: builds the adapters struct %s where it can run more than once: "+
				"&%s{} hangs directly from the body of main, never inside a loop, closure, go statement or label",
				m.pos(f, e), name, written(e)))
		default:
			notAdmitted = append(notAdmitted, fmt.Sprintf("%s: mentions the adapters struct %s other than as &%s{}: "+
				"main builds each adapters struct once, with a literal and its address", m.pos(f, e), name, written(e)))
		}
		return true
	})
	return builds, notAdmitted
}

// addressedAdapters returns the selector pkg.T when n is &pkg.T{} and pkg.T is a struct of an adapters package (see
// adaptersType); nil otherwise.
func addressedAdapters(n ast.Node, adaptersType func(ast.Expr) (*ast.SelectorExpr, string)) *ast.SelectorExpr {
	u, ok := n.(*ast.UnaryExpr)
	if !ok || u.Op != token.AND {
		return nil
	}
	lit, ok := u.X.(*ast.CompositeLit)
	if !ok || lit.Type == nil {
		return nil
	}
	sel, _ := adaptersType(lit.Type)
	return sel
}

// goesBack returns the positions in f of the statements that go back without a loop: every goto and every
// appearance of the identifier main other than the package clause and the name of its own declaration (a call, a
// value, defer or go), which join the loop, the closure, the go statement and the recursion that runsOnce follows by
// the stack.
func goesBack(m *module, f *goFile) []string {
	var out []string
	skip := map[*ast.Ident]bool{f.ast.Name: true} // names that are not a use of main: package, declaration, selected
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncDecl:
			skip[n.Name] = true
		case *ast.SelectorExpr:
			skip[n.Sel] = true
		case *ast.BranchStmt:
			if n.Tok == token.GOTO {
				out = append(out, fmt.Sprintf("%s: uses goto: cmd only wires, and a goto can run main's statements more than "+
					"once", m.pos(f, n)))
			}
		case *ast.Ident:
			if n.Name == "main" && !skip[n] {
				out = append(out, fmt.Sprintf("%s: calls main: cmd only wires, and a main that calls or takes itself as a "+
					"value runs its statements more than once", m.pos(f, n)))
			}
		}
		return true
	})
	return out
}

// written returns pkg.T as written in the selector e.
func written(e *ast.SelectorExpr) string {
	if id, ok := e.X.(*ast.Ident); ok {
		return id.Name + "." + e.Sel.Name
	}
	return e.Sel.Name
}

// runsOnce reports whether a node whose parents are stack (outermost first) runs at most once: it is in the body of
// func main with no loop, range, closure, go statement nor labeled statement between it and that body.
func runsOnce(stack []ast.Node) bool {
	for _, parent := range slices.Backward(stack) {
		switch n := parent.(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.FuncLit, *ast.GoStmt, *ast.LabeledStmt:
			return false
		case *ast.FuncDecl:
			return n.Name.Name == "main" && n.Recv == nil
		}
	}
	return false
}
