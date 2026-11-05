package tests

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-6, the part a check can decide: cmd/ only wires, so its files declare nothing but func main. That the
// entry of a use case only sequences, and takes no decisions, is GOARCH-20, left to the review.
func TestGOARCH06CmdOnlyWires(t *testing.T) {
	checkRule(t, checkCmdOnlyWires, "cmd/app/main.go:5: declares helper")
}

func checkCmdOnlyWires(m *module) []string {
	var out []string
	for _, f := range m.files {
		if f.top != "cmd" || f.kind == kindTest {
			continue
		}
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Name.Name != "main" || d.Recv != nil {
					out = append(out, fmt.Sprintf("%s: declares %s: cmd only wires the commands in main", m.pos(f, d), d.Name.Name))
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
