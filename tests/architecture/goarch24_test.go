package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"testing"
)

// GOARCH-24: the out files, out.go and its siblings out_<topic>.go, hold only the adapter of its use case: the
// public struct Adapter, its methods in any out file, unexported constants, and unexported types that decode or
// encode a format, with their methods; no function, not even a constructor, and no variable. The exported methods
// of the adapter are the methods of the port and no other; what they need goes in unexported methods of the
// adapter, in a decoding type, in a helper of the use case, or in internal/platform/ when it is shared. That an
// unexported type of the out files decodes or encodes a format is left to the review.
func TestGOARCH24(t *testing.T) {
	checkRule(t, checkOutHoldsAdapter,
		"internal/app/order/place/out.go:4: Adapter is a map[string]string, not a struct",
		"internal/app/order/place/out.go:7: declares the function NewAdapter: out.go holds only the struct Adapter",
		"internal/app/order/cancel/out.go:16: Adapter exports Refund, which is not a method of Port",
		"internal/app/order/cancel/out.go:19: declares the function connect",
		"declares the exported constant Timeout",
		"declares the variable cache",
		"internal/app/order/cancel/out_receipt.go:4: declares the exported type Slip: out.go holds only the struct "+
			"Adapter")
	checkAccepts(t, checkOutHoldsAdapter, "internal/app/stock/take/", "internal/app/order/cancel/out.go:7:",
		"internal/app/order/cancel/out.go:23:", "internal/app/order/cancel/out.go:25:",
		"internal/app/order/cancel/out_receipt.go:8:", "internal/app/order/cancel/out_receipt.go:10:")
}

func checkOutHoldsAdapter(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		if uc.files[kindOut] == nil {
			continue // GOARCH-3
		}
		const adapter = "Adapter"
		only := fmt.Sprintf("out.go holds only the struct %s, its methods, unexported constants and unexported "+
			"types that decode or encode a format, without constructor, and so do its siblings", adapter)
		port := portMethods(uc)
		decoders := unexportedTypes(uc.all[kindOut])
		for _, f := range uc.all[kindOut] {
			for _, d := range f.ast.Decls {
				switch d := d.(type) {
				case *ast.FuncDecl:
					switch recv := receiver(d); {
					case recv == "":
						out = append(out, fmt.Sprintf("%s: declares the function %s: %s", m.pos(f, d), d.Name.Name, only))
					case recv == adapter && d.Name.IsExported() && port != nil && !port[d.Name.Name]:
						out = append(out, fmt.Sprintf("%s: %s exports %s, which is not a method of Port", m.pos(f, d),
							adapter, d.Name.Name))
					case recv != adapter && !decoders[recv]:
						out = append(out, fmt.Sprintf("%s: declares the method %s of %s: %s", m.pos(f, d), d.Name.Name,
							recv, only))
					}
				case *ast.GenDecl:
					out = append(out, outGenDecl(m, f, d, adapter, only)...)
				}
			}
		}
	}
	return out
}

// unexportedTypes returns the unexported types that files declare: the decoding types of the out files.
func unexportedTypes(files []*goFile) map[string]bool {
	out := map[string]bool{}
	for _, f := range files {
		for name, what := range decls(f.ast) {
			if what == "type" && !ast.IsExported(name) {
				out[name] = true
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
			switch {
			case s.Name.Name == adapter:
				if _, ok := s.Type.(*ast.StructType); !ok {
					out = append(out, fmt.Sprintf("%s: %s is a %s, not a struct", m.pos(f, s), adapter,
						types.ExprString(s.Type)))
				}
			case s.Name.IsExported():
				out = append(out, fmt.Sprintf("%s: declares the exported type %s: %s", m.pos(f, s), s.Name.Name, only))
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
	spec := typeSpec(in.ast, "Port")
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
