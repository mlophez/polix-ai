package tests

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"
	"testing"
)

// forbiddenImports touch the outside: the domain and the logic never import them. An entry ending in / matches
// any import with that suffix, wherever the module lives.
var forbiddenImports = []string{
	"os", "os/exec", "os/signal", "net", "net/http", "io/fs", "runtime", "syscall",
	"math/rand", "math/rand/v2", "crypto/rand", "log", "log/slog",
	"github.com/spf13/cobra", "/pkg/git", "/pkg/cli",
}

// forbiddenInLogic are imports the domain may use but the logic not: the logic takes no context, it does no
// input nor output to cancel.
var forbiddenInLogic = []string{"context"}

// forbiddenPrint are the functions of fmt that write to the console.
var forbiddenPrint = map[string]bool{"Print": true, "Printf": true, "Println": true}

// forbiddenTime are the functions of time that read the clock or wait.
var forbiddenTime = map[string]bool{
	"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "Tick": true,
	"NewTimer": true, "NewTicker": true, "AfterFunc": true,
}

// GOARCH-4: the domain and the logic are pure: no imports that touch the outside (nor randomness, nor logs, nor
// context in the logic), no clock, no fmt.Print*, no goroutines, no panic and no state of the package (variables,
// except sentinel errors Err* = errors.New or fmt.Errorf); and the logic uses only the domain and its own logic
// file, never the port, the request or the adapter of its use case.
func TestGOARCH04Pure(t *testing.T) {
	checkRule(t, checkPure,
		"internal/order/order.go:4: imports os", "calls time.Now", "the logic uses PlaceOrderPort",
		"imports math/rand", "imports context, which the logic does not need", "calls fmt.Println",
		"starts a goroutine", "calls panic", "declares the variable placeOrderCache")
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
			for _, r := range localRefs(f, syms) {
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
	fmtName := ""
	for name, p := range imports(f.ast) {
		if p == "fmt" {
			fmtName = name
		}
	}
	var out []string
	ast.Inspect(f.ast, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.GoStmt:
			out = append(out, fmt.Sprintf("%s: starts a goroutine", m.pos(f, x)))
		case *ast.CallExpr:
			switch fn := x.Fun.(type) {
			case *ast.Ident:
				if fn.Name == "panic" && fn.Obj == nil {
					out = append(out, fmt.Sprintf("%s: calls panic: return an error", m.pos(f, x)))
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

// packageState returns the variables of the package f declares, but blank ones and sentinel errors.
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
				if id.Name == "_" || (i < len(v.Values) && sentinelError(id.Name, v.Values[i])) {
					continue
				}
				out = append(out, fmt.Sprintf("%s: declares the variable %s: the domain and the logic hold no state",
					m.pos(f, id), id.Name))
			}
		}
	}
	return out
}

// sentinelError reports whether name = value declares a sentinel error: Err* = errors.New(...) or fmt.Errorf(...).
func sentinelError(name string, value ast.Expr) bool {
	if !strings.HasPrefix(name, "Err") && !strings.HasPrefix(name, "err") {
		return false
	}
	call, ok := value.(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && (pkg.Name == "errors" && sel.Sel.Name == "New" || pkg.Name == "fmt" && sel.Sel.Name == "Errorf")
}

func clockCalls(m *module, f *goFile) []string {
	timeName := ""
	for name, p := range imports(f.ast) {
		if p == "time" {
			timeName = name
		}
	}
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
