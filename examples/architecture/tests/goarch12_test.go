package tests

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"
)

// GOARCH-12: a port is one interface, never a struct: the entry declares the interface <Verb><Noun>Port and no
// other interface.
func TestGOARCH12Port(t *testing.T) {
	checkRule(t, checkPort, "PlaceOrderPort is a struct, not an interface",
		"declares the interface Notifier: the only interface is the port PlaceOrderPort")
}

func checkPort(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		in := uc.files[kindIn]
		if in == nil {
			continue // GOARCH-3
		}
		port := camel(uc.stem) + "Port"
		for _, d := range in.ast.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, s := range g.Specs {
				t := s.(*ast.TypeSpec)
				_, isInterface := t.Type.(*ast.InterfaceType)
				switch {
				case t.Name.Name == port && !isInterface:
					out = append(out, fmt.Sprintf("%s: %s is a %s, not an interface", m.pos(in, t), port, kindOfType(t.Type)))
				case t.Name.Name != port && isInterface:
					out = append(out, fmt.Sprintf("%s: declares the interface %s: the only interface is the port %s",
						m.pos(in, t), t.Name.Name, port))
				}
			}
		}
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
