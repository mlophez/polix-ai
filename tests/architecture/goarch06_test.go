package architecture

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-6: cmd/ only wires, so its files declare nothing but func main, and it has no tests: the commands are
// tested through their entries in internal/.
func TestGOARCH6(t *testing.T) {
	checkRule(t, checkCmdOnlyWires, "cmd/app/main.go: declares helper",
		"cmd/app/main_test.go: cmd has no tests")
}

func checkCmdOnlyWires(m *module) []string {
	var out []string
	for _, f := range m.files {
		if f.top != "cmd" {
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
	}
	return out
}
