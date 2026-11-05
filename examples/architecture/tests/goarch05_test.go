package tests

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-5: the adapter implements the port of its own use case: <Verb><Noun>Adapter has every method of
// <Verb><Noun>Port.
func TestGOARCH05Adapter(t *testing.T) {
	checkRule(t, checkAdapter,
		"CancelOrderAdapter does not implement Cancel of CancelOrderPort")
}

func checkAdapter(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		in, adapterFile := uc.files[kindIn], uc.files[kindOut]
		if in == nil || adapterFile == nil {
			continue // GOARCH-3
		}
		port, adapter := camel(uc.stem)+"Port", camel(uc.stem)+"Adapter"
		spec := typeSpec(in.ast, port)
		if spec == nil || typeSpec(adapterFile.ast, adapter) == nil {
			continue // GOARCH-11
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			continue // GOARCH-12
		}
		have := methods(adapterFile.ast, adapter)
		for _, f := range iface.Methods.List {
			for _, name := range f.Names {
				if !have[name.Name] {
					out = append(out, fmt.Sprintf("%s: %s does not implement %s of %s", adapterFile.rel, adapter, name.Name, port))
				}
			}
		}
	}
	return out
}

// methods returns the names of the methods of the type name declared in f.
func methods(f *ast.File, name string) map[string]bool {
	out := map[string]bool{}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		recv := fn.Recv.List[0].Type
		if star, ok := recv.(*ast.StarExpr); ok {
			recv = star.X
		}
		if id, ok := recv.(*ast.Ident); ok && id.Name == name {
			out[fn.Name.Name] = true
		}
	}
	return out
}
