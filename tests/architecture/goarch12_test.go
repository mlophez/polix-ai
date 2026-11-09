package architecture

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-12: a port is one interface, never a struct: in.go declares the interface Port. Besides it, the in files
// (in.go and its siblings in_<topic>.go) declare at most one interface, unexported and named, of the transport
// formats the entry reads (format); no other interface, named or anonymous. The first unexported named interface of
// the in files, in the order of their names, is the one of the formats.
func TestGOARCH12(t *testing.T) {
	checkRule(t, checkPort, "internal/app/order/place/in.go:4: Port is a struct, not an interface",
		"internal/app/order/place/in.go:7: declares the interface Notifier: the only interface of in.go is Port",
		"internal/app/order/cancel/in.go:35: declares an anonymous interface: the only interface of in.go is Port",
		"internal/app/stock/count/in_scan.go:7: declares the interface source: the only interface of in.go is Port, "+
			"and the in files declare besides it one unexported interface of the formats the entry reads, here format")
	checkAccepts(t, checkPort, "internal/app/stock/take/", "internal/app/stock/count/in_scan.go:4")
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
		only := fmt.Sprintf("the only interface of in.go is %s, and the in files declare besides it one unexported "+
			"interface of the formats the entry reads", port)
		format := ""
		for _, f := range uc.all[kindIn] {
			named := namedInterfaces(f.ast)
			ast.Inspect(f.ast, func(n ast.Node) bool {
				it, ok := n.(*ast.InterfaceType)
				if !ok || (spec != nil && it == spec.Type) {
					return true
				}
				name, ok := named[it]
				switch {
				case !ok:
					out = append(out, fmt.Sprintf("%s: declares an anonymous interface: %s", m.pos(f, it), only))
				case format == "" && !ast.IsExported(name):
					format = name
				case format != "":
					out = append(out, fmt.Sprintf("%s: declares the interface %s: %s, here %s", m.pos(f, it), name, only,
						format))
				default:
					out = append(out, fmt.Sprintf("%s: declares the interface %s: %s", m.pos(f, it), name, only))
				}
				return true
			})
		}
	}
	return out
}

// namedInterfaces returns the interfaces that f declares as named types, by their name.
func namedInterfaces(f *ast.File) map[*ast.InterfaceType]string {
	named := map[*ast.InterfaceType]string{}
	for _, d := range f.Decls {
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
	return named
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
