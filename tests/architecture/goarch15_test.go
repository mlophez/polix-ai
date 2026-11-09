package architecture

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-15: the only exported function of in.go is the entry Command.
func TestGOARCH15(t *testing.T) {
	checkRule(t, checkOnlyCommandExported,
		"internal/app/order/place/in.go:13: exports Helper: the only exported function is Command")
}

func checkOnlyCommandExported(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue // GOARCH-3
		}
		command := "Command"
		for _, d := range in.ast.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.IsExported() && fn.Name.Name != command {
				out = append(out, fmt.Sprintf("%s: exports %s: the only exported function is %s", m.pos(in, fn),
					fn.Name.Name, command))
			}
		}
	}
	return out
}
