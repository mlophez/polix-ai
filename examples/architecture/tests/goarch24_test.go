package tests

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

// GOARCH-24: the out file holds only the adapter of its use case: the public struct <Verb><Noun>Adapter, its
// methods and unexported constants; no function, not even a constructor. The exported methods of the adapter are
// the methods of the port and no other; what they need goes in unexported methods of the adapter, or in pkg/ when
// it is shared.
func TestGOARCH24OutHoldsTheAdapter(t *testing.T) {
	checkRule(t, checkOutHoldsAdapter,
		"internal/order/place_order_out.go:4: PlaceOrderAdapter is a map[string]string, not a struct",
		"declares the function NewPlaceOrderAdapter: the out file holds only the adapter PlaceOrderAdapter",
		"CancelOrderAdapter exports Refund, which is not a method of CancelOrderPort",
		"declares the function connect: the out file holds only the adapter CancelOrderAdapter",
		"declares the exported constant Timeout",
		"declares the variable cache",
		"declares the type options")
}

func checkOutHoldsAdapter(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		f := uc.files[kindOut]
		if f == nil {
			continue // GOARCH-3
		}
		adapter := camel(uc.stem) + "Adapter"
		only := fmt.Sprintf("the out file holds only the adapter %s, its methods and unexported constants, "+
			"without constructor", adapter)
		port := portMethods(uc)
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				switch recv := receiver(d); {
				case recv == "":
					out = append(out, fmt.Sprintf("%s: declares the function %s: %s", m.pos(f, d), d.Name.Name, only))
				case recv != adapter:
					out = append(out, fmt.Sprintf("%s: declares the method %s of %s: %s", m.pos(f, d), d.Name.Name, recv,
						only))
				case recv == adapter && d.Name.IsExported() && port != nil && !port[d.Name.Name]:
					out = append(out, fmt.Sprintf("%s: %s exports %s, which is not a method of %sPort", m.pos(f, d),
						adapter, d.Name.Name, camel(uc.stem)))
				}
			case *ast.GenDecl:
				out = append(out, outGenDecl(m, f, d, adapter, only)...)
			}
		}
	}
	return out
}

// outGenDecl checks the types, constants and variables an out file declares.
func outGenDecl(m *module, f *goFile, d *ast.GenDecl, adapter, only string) []string {
	var out []string
	for _, s := range d.Specs {
		switch s := s.(type) {
		case *ast.TypeSpec:
			if s.Name.Name != adapter {
				out = append(out, fmt.Sprintf("%s: declares the type %s: %s", m.pos(f, s), s.Name.Name, only))
			} else if _, ok := s.Type.(*ast.StructType); !ok {
				out = append(out, fmt.Sprintf("%s: %s is a %s, not a struct", m.pos(f, s), adapter,
					types.ExprString(s.Type)))
			}
		case *ast.ValueSpec:
			for _, id := range s.Names {
				switch {
				case d.Tok == token.VAR:
					out = append(out, fmt.Sprintf("%s: declares the variable %s: %s", m.pos(f, id), id.Name, only))
				case id.IsExported():
					out = append(out, fmt.Sprintf("%s: declares the exported constant %s: %s", m.pos(f, id), id.Name, only))
				}
			}
		}
	}
	return out
}

// portMethods returns the methods of the port of uc, or nil when its entry does not declare it as an interface.
func portMethods(uc *useCase) map[string]bool {
	in := uc.files[kindIn]
	if in == nil {
		return nil
	}
	spec := typeSpec(in.ast, camel(uc.stem)+"Port")
	if spec == nil {
		return nil
	}
	iface, ok := spec.Type.(*ast.InterfaceType)
	if !ok {
		return nil
	}
	out := map[string]bool{}
	for _, f := range iface.Methods.List {
		for _, name := range f.Names {
			out[name.Name] = true
		}
	}
	return out
}

// receiver returns the name of the type of the receiver of fn, or "" for a function.
func receiver(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return types.ExprString(t)
}
