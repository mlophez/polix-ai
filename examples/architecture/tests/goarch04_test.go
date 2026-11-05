package tests

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"
)

// forbiddenImports touch the outside: the domain and the logic never import them. An entry ending in / matches
// any import with that suffix, wherever the module lives.
var forbiddenImports = []string{
	"os", "os/exec", "os/signal", "net", "net/http", "io/fs", "runtime", "syscall",
	"github.com/spf13/cobra", "/pkg/git", "/pkg/cli",
}

// forbiddenTime are the functions of time that read the clock or wait.
var forbiddenTime = map[string]bool{
	"Now": true, "Since": true, "Until": true, "Sleep": true, "After": true, "Tick": true,
	"NewTimer": true, "NewTicker": true, "AfterFunc": true,
}

// GOARCH-4: the domain and the logic are pure: no imports that touch the outside, no clock, and the logic uses
// only the domain and its own logic file, never the port, the request or the adapter of its use case.
func TestGOARCH04Pure(t *testing.T) {
	checkRule(t, checkPure,
		"internal/order/order.go:4: imports os", "calls time.Now", "the logic uses PlaceOrderPort")
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
	}
	return out
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
