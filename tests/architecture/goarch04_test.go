package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// forbiddenImports touch the outside: the domain and the logic never import them. An entry ending in / matches
// any import with that suffix, wherever the module lives.
var forbiddenImports = []string{
	"os", "os/exec", "os/signal", "net", "net/http", "io/fs", "runtime", "syscall",
	"math/rand", "math/rand/v2", "crypto/rand", "log", "log/slog",
	"github.com/spf13/cobra", "/pkg/git", "/pkg/cli", "/pkg/proc", "/pkg/github", "/pkg/jsonl", "/pkg/build",
	"/pkg/file",
}

// forbiddenInLogic are imports the domain may use but the logic not: the logic takes no context, it does no
// input nor output to cancel.
var forbiddenInLogic = []string{"context"}

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

// GOARCH-4: the domain and the logic are pure: no imports that touch the outside (nor randomness, nor logs, nor
// context in the logic), no clock, no fmt.Print*, print nor println, no goroutines, no panic and no state of the
// package: a variable is a table built once in its declaration (literals, regexp.MustCompile, other tables) and
// never assigned, indexed-assigned nor appended to afterwards, a blank one, or a sentinel error Err* = errors.New
// or fmt.Errorf; and the logic uses only the domain and its own logic file, never the port, the request or the
// adapter of its use case.
func TestGOARCH4(t *testing.T) {
	t.Run("no state", sentinelErrorsAndTablesAreNoState)
	checkRule(t, checkPure,
		"internal/order/order.go:4: imports os", "calls time.Now", "the logic uses PlaceOrderPort",
		"imports math/rand", "imports context, which the logic does not need", "calls fmt.Println",
		"starts a goroutine", "calls panic", "imports log/slog", "pkg/git, which touches the outside",
		"calls println, which writes to the console",
		"internal/cart/cart_tax.go:6: imports example.com/app/pkg/proc, which touches the outside",
		"internal/cart/cart_disk.go:6: imports example.com/app/pkg/file, which touches the outside",
		"the variable placeOrderCache is state: internal/order/place_order_logic.go",
		"the variable placeOrderSeen is state: its value is not a table",
		"the variable placeOrderClock is state: its value is not a table")
}

func checkPure(m *module) []string {
	var out []string
	for _, files := range m.packages() {
		syms := symbols(files)
		for _, f := range files {
			if f.kind != kindDomain && f.kind != kindLogic {
				continue
			}
			out = append(out, impureImports(m, f)...)
			out = append(out, clockCalls(m, f)...)
			out = append(out, effects(m, f)...)
			out = append(out, packageState(m, f)...)
			if f.kind != kindLogic {
				continue
			}
			for _, r := range m.localRefs(f, syms) {
				if r.decl.kind != kindDomain && (r.decl.kind != kindLogic || r.decl.useCase != f.useCase) {
					out = append(out, fmt.Sprintf("%s: the logic uses %s, of %s", m.pos(f, r.node), r.name, r.decl.name))
				}
			}
		}
	}
	return out
}

func impureImports(m *module, f *goFile) []string {
	var out []string
	for _, s := range f.ast.Imports {
		p := strings.Trim(s.Path.Value, `"`)
		for _, bad := range forbiddenImports {
			if p == bad || (strings.HasPrefix(bad, "/") && strings.HasSuffix(p, bad)) {
				out = append(out, fmt.Sprintf("%s: imports %s, which touches the outside", m.pos(f, s), p))
			}
		}
		if f.kind == kindLogic && contains(forbiddenInLogic, p) {
			out = append(out, fmt.Sprintf("%s: imports %s, which the logic does not need", m.pos(f, s), p))
		}
	}
	return out
}

// effects returns the writes to the console, goroutines and panics of f.
func effects(m *module, f *goFile) []string {
	fmtName := importedAs(f.ast, "fmt")
	var out []string
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			out = append(out, fmt.Sprintf("%s: starts a goroutine", m.pos(f, x)))
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
		for _, s := range g.Specs {
			v := s.(*ast.ValueSpec)
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
	f, err := parser.ParseFile(m.fset, "internal/rule/rule.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	g := &goFile{rel: "internal/rule/rule.go", top: "internal", dir: "internal/rule", pkg: "rule", name: "rule.go",
		kind: kindDomain, ast: f}
	m.files = append(m.files, g)
	if got := packageState(m, g); len(got) > 0 {
		t.Fatalf("sentinel errors and tables reported as state: %v", got)
	}
}
