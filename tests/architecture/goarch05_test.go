package architecture

import (
	"fmt"
	"go/ast"
	"go/types"
	"maps"
	"testing"
)

// GOARCH-5: the adapter implements the port of its own use case: Adapter of out.go has every method of Port of
// in.go, declared in any out file (out.go or a sibling out_<topic>.go) or promoted from an embedded field. The
// checker decides it when the adapter and the port resolve; else the methods declared in the out files are
// compared by name.
func TestGOARCH5(t *testing.T) {
	checkRule(t, checkAdapter,
		"internal/app/order/cancel/out.go: Adapter does not implement Cancel of Port, declared nor promoted")
	checkAccepts(t, checkAdapter, "internal/app/stock/take/")
	t.Run("by name", func(t *testing.T) {
		m := mustLoad(t, badRoot)
		var take *useCase
		for _, uc := range m.useCases() {
			if uc.dir == "internal/app/stock/take" {
				take = uc
			}
		}
		if take == nil {
			t.Fatal("testdata/bad has no use case internal/app/stock/take")
		}
		if have := adapterMethods(take); !have["Load"] {
			t.Errorf("the method Load of a sibling of out.go is not found by name: %v", have)
		}
	})
}

func checkAdapter(m *module) []string {
	info := m.typed()
	var out []string
	for _, uc := range m.useCases() {
		in, adapterFile := uc.files[kindIn], uc.files[kindOut]
		if in == nil || adapterFile == nil {
			continue // GOARCH-3
		}
		port, adapter := "Port", "Adapter"
		spec, adapterSpec := typeSpec(in.ast, port), typeSpec(adapterFile.ast, adapter)
		if spec == nil || adapterSpec == nil {
			continue // GOARCH-11
		}
		iface, ok := spec.Type.(*ast.InterfaceType)
		if !ok {
			continue // GOARCH-12
		}
		if missing, decided := missingByTypes(info, spec, adapterSpec); decided {
			for _, name := range missing {
				out = append(out, fmt.Sprintf("%s: %s does not implement %s of %s, declared nor promoted", adapterFile.rel,
					adapter, name, port))
			}
			continue
		}
		have := adapterMethods(uc)
		for _, f := range iface.Methods.List {
			for _, name := range f.Names {
				if !have[name.Name] {
					out = append(out, fmt.Sprintf("%s: %s declares no method %s of %s", adapterFile.rel, adapter, name.Name,
						port))
				}
			}
		}
	}
	return out
}

// missingByTypes asks the checker which methods of the port the adapter lacks. It decides nothing when a type
// did not resolve, or when the adapter embeds a type that did not resolve, whose promoted methods are unknown.
func missingByTypes(info *types.Info, port, adapter *ast.TypeSpec) ([]string, bool) {
	p, a := info.Defs[port.Name], info.Defs[adapter.Name]
	if p == nil || a == nil {
		return nil, false
	}
	iface, ok := p.Type().Underlying().(*types.Interface)
	if !ok {
		return nil, false
	}
	st, ok := adapter.Type.(*ast.StructType)
	if !ok {
		return nil, false
	}
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 && invalid(info.TypeOf(f.Type)) {
			return nil, true // embedded, and nobody here knows its methods: the compiler decides
		}
	}
	var missing []string
	for mth := range iface.Methods() {
		if obj, _, _ := types.LookupFieldOrMethod(a.Type(), true, a.Pkg(), mth.Name()); obj == nil {
			missing = append(missing, mth.Name())
		}
	}
	return missing, true
}

// invalid reports whether t, through pointers, is a type the checker could not resolve.
func invalid(t types.Type) bool {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	b, ok := t.(*types.Basic)
	return ok && b.Kind() == types.Invalid
}

// adapterMethods returns the names of the methods of Adapter declared in the out files of uc.
func adapterMethods(uc *useCase) map[string]bool {
	out := map[string]bool{}
	for _, f := range uc.all[kindOut] {
		maps.Copy(out, methods(f.ast, "Adapter"))
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
		if receiver(fn) == name {
			out[fn.Name.Name] = true
		}
	}
	return out
}
