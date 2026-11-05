package tests

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-12: a port is one interface, never a struct: the in file declares the interface <Verb><Noun>Port and no
// other interface, named or anonymous.
func TestGOARCH12Port(t *testing.T) {
	checkRule(t, checkPort, "PlaceOrderPort is a struct, not an interface",
		"declares the interface Notifier: the only interface is the port PlaceOrderPort",
		"declares an anonymous interface: the only interface is the port CancelOrderPort")
}

func checkPort(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue // GOARCH-3
		}
		port := camel(uc.stem) + "Port"
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
				out = append(out, fmt.Sprintf("%s: declares the interface %s: the only interface is the port %s",
					m.pos(in, it), name, port))
			} else {
				out = append(out, fmt.Sprintf("%s: declares an anonymous interface: the only interface is the port %s",
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
