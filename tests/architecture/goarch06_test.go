package architecture

import (
	"fmt"
	"go/ast"
	"sort"
	"testing"
)

// GOARCH-6: cmd/ only wires, so its files declare nothing but func main, and it has no tests: the commands are
// tested through their entries in internal/. main builds the loader of each load use case once, calling its
// Command once, and passes it to the Command of every use case that reads its file.
func TestGOARCH6(t *testing.T) {
	checkRule(t, checkCmdOnlyWires, "cmd/app/main.go: declares helper",
		"cmd/app/main_test.go: cmd has no tests",
		"cmd/app: builds the loader of internal/app/stock/load 2 times")
}

func checkCmdOnlyWires(m *module) []string {
	var out []string
	builds := map[string]map[string]int{} // by folder of cmd/, the calls of the Command of each load use case
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
		for dir, n := range loaderBuilds(m, f) {
			builds[f.dir][dir] += n
		}
	}
	for cmd, byLoader := range builds {
		for dir, n := range byLoader {
			if n > 1 {
				out = append(out, fmt.Sprintf("%s: builds the loader of %s %d times: main builds each loader once "+
					"and passes it to the Command of every use case that reads its file", cmd, dir, n))
			}
		}
	}
	sort.Strings(out)
	return out
}

// loaderBuilds counts the calls of f to the entry Command of each load use case it imports, by its folder
// (internal/app/<noun>/load).
func loaderBuilds(m *module, f *goFile) map[string]int {
	loaders := map[string]string{} // local name of the import -> folder of the load use case
	for name, p := range imports(f.ast) {
		if dir := m.moduleDir(p); placeOf(dir).level == levelUseCase && placeOf(dir).verb == loadVerb {
			loaders[name] = dir
		}
	}
	out := map[string]int{}
	ast.Inspect(f.ast, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Command" {
			if id, ok := sel.X.(*ast.Ident); ok && loaders[id.Name] != "" {
				out[loaders[id.Name]]++
			}
		}
		return true
	})
	return out
}
