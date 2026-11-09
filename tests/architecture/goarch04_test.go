package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strings"
	"testing"
)

// forbiddenPrint are the functions of fmt that write to the console.
var forbiddenPrint = map[string]bool{"Print": true, "Printf": true, "Println": true}

// forbiddenBuiltins are the builtins that write to the console or panic.
var forbiddenBuiltins = map[string]string{
	"print": "writes to the console", "println": "writes to the console", "panic": "panics: return an error",
}

// forbiddenTime are the functions of time that read the clock or wait.
var forbiddenTime = map[string]bool{
	"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "Tick": true,
	"NewTimer": true, "NewTicker": true, "AfterFunc": true,
}

// GOARCH-4: the domain and the logic are pure: no clock, no fmt.Print*, print nor println, no goroutines, no panic
// and no state of the package: a variable is a table built once in its declaration (literals, regexp.MustCompile,
// other tables) and never assigned, indexed-assigned nor appended to afterwards, a blank one, or a sentinel error
// Err* = errors.New or fmt.Errorf. The logic files, logic.go and its siblings logic_<topic>.go, use no name
// declared in the in files nor the out files, and a helper the logic imports is as pure as the logic: the same
// checks, and only the imports that the list GOARCH-41-logic of .golangci.yml allows. The imports of the domain
// and the logic themselves are GOARCH-41, which depguard decides.
func TestGOARCH4(t *testing.T) {
	t.Run("no state", sentinelErrorsAndTablesAreNoState)
	checkRule(t, checkPure,
		"calls time.Now", "the logic uses Port, of in.go", "calls fmt.Println", "starts a goroutine", "calls panic",
		"calls println, which writes to the console",
		"the variable cache is state: internal/app/order/place/logic.go",
		"the variable seen is state: its value is not a table",
		"the variable clock is state: its value is not a table",
		"internal/app/order/place/pricing/pricing.go:7: imports os, out of the list GOARCH-41-logic: a helper "+
			"the logic internal/app/order/place/logic.go imports",
		"internal/app/order/place/pricing/pricing.go:17: calls time.Now, which reads the clock: a helper the logic "+
			"internal/app/order/place/logic.go imports",
		"internal/app/stock/count/logic_audit.go:12: calls time.Now, which reads the clock",
		"internal/app/stock/count/logic_audit.go:10: the logic uses box, of in.go")
	checkAccepts(t, checkPure, "internal/app/stock/take/")
}

func checkPure(m *module) []string {
	var out []string
	list, listModule, err := readDepguardList("GOARCH-41-logic")
	if err != nil {
		out = append(out, err.Error())
	}
	for _, files := range m.packages() {
		syms := symbols(files)
		for _, f := range files {
			if f.kind != kindDomain && f.kind != kindLogic {
				continue
			}
			out = append(out, pure(m, f)...)
			if f.kind != kindLogic {
				continue
			}
			for _, r := range m.localRefs(f, syms) {
				if r.decl.kind != kindLogic {
					out = append(out, fmt.Sprintf("%s: the logic uses %s, of %s", m.pos(f, r.node), r.name, r.decl.name))
				}
			}
			out = append(out, logicHelpers(m, f, list, listModule)...)
		}
	}
	return out
}

// pure returns the reads of the clock, the effects and the state of the package of f.
func pure(m *module, f *goFile) []string {
	out := clockCalls(m, f)
	out = append(out, effects(m, f)...)
	return append(out, packageState(m, f)...)
}

// logicHelpers checks the helpers of its use case that the logic file logic, main or sibling, imports: each of
// their files is pure and imports only what list, the list GOARCH-41-logic of depguard, allows. listModule is the
// module path its entries name.
func logicHelpers(m *module, logic *goFile, list depguardList, listModule string) []string {
	var out []string
	for _, s := range logic.ast.Imports {
		dir := m.moduleDir(strings.Trim(s.Path.Value, `"`))
		if placeOf(dir).level != levelHelper || path.Dir(dir) != logic.dir {
			continue // a helper of another use case is GOARCH-9
		}
		why := fmt.Sprintf(": a helper the logic %s imports", logic.rel)
		for _, f := range m.files {
			if f.dir != dir || f.kind == kindTest {
				continue
			}
			for _, imp := range f.ast.Imports {
				if p := strings.Trim(imp.Path.Value, `"`); !list.allows(m.inModule(p, listModule)) {
					out = append(out, fmt.Sprintf("%s: imports %s, out of the list GOARCH-41-logic%s", m.pos(f, imp), p,
						why))
				}
			}
			for _, v := range pure(m, f) {
				out = append(out, v+why)
			}
		}
	}
	return out
}

// inModule returns the import p of the module under check as an import of the module whose path is to, so the
// entries of a list of depguard that name to match it: example.com/app/internal/app/rule is
// github.com/mlophez/polix-ai/internal/app/rule. Any other import is returned as it is.
func (m *module) inModule(p, to string) string {
	if m.path != "" {
		if rest, ok := strings.CutPrefix(p, m.path+"/"); ok {
			return to + "/" + rest
		}
	}
	if dir := m.moduleDir(p); dir != "" {
		return to + "/" + dir
	}
	return p
}

// effects returns the writes to the console, goroutines and panics of f.
func effects(m *module, f *goFile) []string {
	fmtName := importedAs(f.ast, "fmt")
	var out []string
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			out = append(out, m.pos(f, x)+": starts a goroutine")
		case *ast.CallExpr:
			switch fn := x.Fun.(type) {
			case *ast.Ident:
				if why, ok := forbiddenBuiltins[fn.Name]; ok && fn.Obj == nil {
					out = append(out, fmt.Sprintf("%s: calls %s, which %s", m.pos(f, x), fn.Name, why))
				}
			case *ast.SelectorExpr:
				if id, ok := fn.X.(*ast.Ident); ok && fmtName != "" && id.Name == fmtName && id.Obj == nil &&
					forbiddenPrint[fn.Sel.Name] {
					out = append(out, fmt.Sprintf("%s: calls fmt.%s, which writes to the console", m.pos(f, x),
						fn.Sel.Name))
				}
			}
		}
		return true
	})
	return out
}

// packageState returns the variables of the package f declares that are state: neither blank, nor a sentinel
// error, nor a table (a value built from literals, regexp.MustCompile or other tables that nothing of the
// package writes afterwards).
func packageState(m *module, f *goFile) []string {
	var out []string
	for _, d := range f.ast.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			continue
		}
		for _, v := range valueSpecs(g) {
			for i, id := range v.Names {
				if id.Name == "_" {
					continue
				}
				var value ast.Expr
				if i < len(v.Values) {
					value = v.Values[i]
				}
				if sentinelError(f.ast, id.Name, value) {
					continue
				}
				switch {
				case !table(f.ast, value):
					out = append(out, fmt.Sprintf("%s: the variable %s is state: its value is not a table of literals",
						m.pos(f, id), id.Name))
				default:
					for _, w := range m.writesTo(f.dir, id) {
						out = append(out, fmt.Sprintf("%s: the variable %s is state: %s writes it", m.pos(f, id), id.Name, w))
					}
				}
			}
		}
	}
	return out
}

// sentinelError reports whether name = value declares a sentinel error: Err* = errors.New(...) or fmt.Errorf(...).
func sentinelError(f *ast.File, name string, value ast.Expr) bool {
	if !strings.HasPrefix(name, "Err") && !strings.HasPrefix(name, "err") {
		return false
	}
	return callTo(f, value, "errors", "New") || callTo(f, value, "fmt", "Errorf")
}

// callTo reports whether e is a call pkg.fn, with pkg the import path.
func callTo(f *ast.File, e ast.Expr, pkg, fn string) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Obj == nil && imports(f)[id.Name] == pkg && sel.Sel.Name == fn
}

// table reports whether value is built once from literals: a literal, a composite literal (also through &), a
// regexp.MustCompile of one, or a name (another table or a constant).
func table(f *ast.File, value ast.Expr) bool {
	switch v := value.(type) {
	case nil:
		return false
	case *ast.BasicLit, *ast.Ident:
		return true
	case *ast.ParenExpr:
		return table(f, v.X)
	case *ast.UnaryExpr:
		return v.Op == token.AND && table(f, v.X)
	case *ast.CompositeLit:
		for _, e := range v.Elts {
			if kv, ok := e.(*ast.KeyValueExpr); ok {
				e = kv.Value
			}
			if !table(f, e) {
				return false
			}
		}
		return true
	case *ast.CallExpr:
		return callTo(f, v, "regexp", "MustCompile") && len(v.Args) == 1 && table(f, v.Args[0])
	}
	return false
}

// writesTo returns where the files of the package dir write the package variable name declared by decl: an
// assignment to it or to an element or field of it, an increment, or an append to it.
func (m *module) writesTo(dir string, decl *ast.Ident) []string {
	info := m.typed()
	obj := info.Defs[decl]
	var out []string
	for _, f := range m.files {
		if f.dir != dir {
			continue
		}
		refers := func(e ast.Expr) bool {
			id, ok := rootIdent(e)
			if !ok || id.Name != decl.Name {
				return false
			}
			if use, ok := info.Uses[id]; ok {
				return obj != nil && use == obj
			}
			return id.Obj == nil || id.Obj == decl.Obj // not typed (a test): not shadowed in its file
		}
		ast.Inspect(f.ast, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.AssignStmt:
				for _, l := range x.Lhs {
					if refers(l) {
						out = append(out, m.pos(f, x))
					}
				}
			case *ast.IncDecStmt:
				if refers(x.X) {
					out = append(out, m.pos(f, x))
				}
			case *ast.CallExpr:
				if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "append" && id.Obj == nil && len(x.Args) > 0 &&
					refers(x.Args[0]) {
					out = append(out, m.pos(f, x))
				}
			}
			return true
		})
	}
	return out
}

// rootIdent returns the identifier at the root of x.F, x[i], *x and (x), or false.
func rootIdent(e ast.Expr) (*ast.Ident, bool) {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			return x, true
		case *ast.SelectorExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil, false
		}
	}
}

func clockCalls(m *module, f *goFile) []string {
	timeName := importedAs(f.ast, "time")
	if timeName == "" {
		return nil
	}
	var out []string
	ast.Inspect(f.ast, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == timeName && id.Obj == nil && forbiddenTime[sel.Sel.Name] {
				out = append(out, fmt.Sprintf("%s: calls time.%s, which reads the clock", m.pos(f, sel), sel.Sel.Name))
			}
		}
		return true
	})
	return out
}

// sentinelErrorsAndTablesAreNoState checks that a sentinel error and a table are no state: the exception of
// packageState holds. TestGOARCH4 runs it, so the run of the rule covers it.
func sentinelErrorsAndTablesAreNoState(t *testing.T) {
	src := `package rule
import ("errors"; "fmt"; "regexp")
var ErrEmpty = errors.New("empty")
var errLate = fmt.Errorf("late: %w", ErrEmpty)
var kinds = map[string]bool{"a": true}
var ids = regexp.MustCompile("^[A-Z]+-[0-9]+$")
var all = []string{"a", "b"}
var _ = all
`
	m := &module{fset: token.NewFileSet()}
	f, err := parser.ParseFile(m.fset, "internal/app/rule/rule.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &goFile{rel: "internal/app/rule/rule.go", dir: "internal/app/rule", pkg: "rule", name: "rule.go",
		place: placeOf("internal/app/rule"), kind: kindDomain, ast: f}
	m.files = append(m.files, g)
	if got := packageState(m, g); len(got) > 0 {
		t.Fatalf("sentinel errors and tables reported as state: %v", got)
	}
}
