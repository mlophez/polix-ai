package architecture

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-12: a port is one interface, never a struct: in.go declares the interface Port and no other interface,
// named or anonymous.
func TestGOARCH12(t *testing.T) {
	checkRule(t, checkPort, "internal/app/order/place/in.go:4: Port is a struct, not an interface",
		"internal/app/order/place/in.go:7: declares the interface Notifier: the only interface of in.go is Port",
		"internal/app/order/cancel/in.go:35: declares an anonymous interface: the only interface of in.go is Port")
}

func checkPort(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue // GOARCH-3
		}
		port := "Port"
		spec := typeSpec(in.ast, port)
		if spec != nil {
			if _, isInterface := spec.Type.(*ast.InterfaceType); !isInterface {
				out = append(out, fmt.Sprintf("%s: %s is a %s, not an interface", m.pos(in, spec), port,
					kindOfType(spec.Type)))
			}
		}
		named := map[*ast.InterfaceType]string{}
		for _, d := range in.ast.Decls {
			if g, ok := d.(*ast.GenDecl); ok {
				for _, s := range g.Specs {
					if t, ok := s.(*ast.TypeSpec); ok {
						if it, ok := t.Type.(*ast.InterfaceType); ok {
							named[it] = t.Name.Name
						}
					}
				}
			}
		}
		ast.Inspect(in.ast, func(n ast.Node) bool {
			it, ok := n.(*ast.InterfaceType)
			if !ok || (spec != nil && it == spec.Type) {
				return true
			}
			if name, ok := named[it]; ok {
				out = append(out, fmt.Sprintf("%s: declares the interface %s: the only interface of in.go is %s",
					m.pos(in, it), name, port))
			} else {
				out = append(out, fmt.Sprintf("%s: declares an anonymous interface: the only interface of in.go is %s",
					m.pos(in, it), port))
			}
			return true
		})
	}
	return out
}

// kindOfType names the kind of a type expression for a message.
func kindOfType(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.FuncType:
		return "func"
	case *ast.MapType:
		return "map"
	case *ast.ArrayType:
		return "slice"
	}
	return "named type"
}
