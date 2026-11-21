package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"
)

// GOARCH-46: an adapters package, internal/app/<noun>/adapters/, declares only exported structs, with their methods,
// and unexported types, constants and helpers: no exported function, constant, variable, interface or function type, no
// method of an exported type that is not a struct, and no name reserved to the use cases (GOARCH-23), as a type or as a
// method. The exported methods of an unexported type, such as UnmarshalYAML of a decoding type, are admitted.
func TestGOARCH46(t *testing.T) {
	checkRule(t, checkAdaptersDeclarations,
		"internal/app/order/adapters/file_faults.go:5: declares the exported function Open",
		"internal/app/order/adapters/file_faults.go:8: declares the exported constant Mode",
		"internal/app/order/adapters/file_faults.go:11: declares the exported variable Default",
		"internal/app/order/adapters/file_faults.go:14: declares the exported interface Reader",
		"internal/app/order/adapters/file_faults.go:17: declares the exported function type Hook",
		"internal/app/order/adapters/file_faults.go:20: declares the method Run of Hook, an exported type that is not a "+
			"struct",
		"internal/app/order/adapters/file_faults.go:23: declares Adapter, a name reserved to the use cases",
		"internal/app/order/adapters/file_faults.go:26: declares Decide, a name reserved to the use cases",
		"internal/app/order/adapters/file_faults.go:29: declares Command, a name reserved to the use cases")
	checkAccepts(t, checkAdaptersDeclarations, "internal/app/order/adapters/file.go", "internal/app/stock/adapters/log.go")
}

func checkAdaptersDeclarations(m *module) []string {
	structs := adaptersStructs(m)
	var out []string
	for _, f := range m.sources() {
		if f.place.level != levelAdapters {
			continue
		}
		for _, d := range f.ast.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				out = append(out, adaptersFunc(m, f, d, structs)...)
			case *ast.GenDecl:
				out = append(out, adaptersGen(m, f, d)...)
			}
		}
	}
	return out
}

// adaptersStructs returns the types of the adapters packages, as folder.Name, true for a struct.
func adaptersStructs(m *module) map[string]bool {
	out := map[string]bool{}
	for _, f := range m.sources() {
		if f.place.level != levelAdapters {
			continue
		}
		for _, d := range f.ast.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, t := range typeSpecs(g) {
					_, isStruct := t.Type.(*ast.StructType)
					out[f.dir+"."+t.Name.Name] = isStruct
				}
			}
		}
	}
	return out
}

// adaptersFunc reports the function or method d of an adapters package: an exported function, a name reserved to the
// use cases, or an exported method of an exported type that is not a struct.
func adaptersFunc(m *module, f *goFile, d *ast.FuncDecl, structs map[string]bool) []string {
	name := d.Name.Name
	switch {
	case reserved(name):
		return []string{reservedAdapters(m, f, d.Name)}
	case !d.Name.IsExported():
		return nil
	case d.Recv == nil:
		return []string{fmt.Sprintf("%s: declares the exported function %s: an adapters package declares only exported "+
			"structs, with their methods", m.pos(f, d.Name), name)}
	}
	recv := receiverName(d)
	if isStruct, known := structs[f.dir+"."+recv]; known && !isStruct && ast.IsExported(recv) {
		return []string{fmt.Sprintf("%s: declares the method %s of %s, an exported type that is not a struct: an adapters "+
			"package declares only exported structs, with their methods", m.pos(f, d.Name), name, recv)}
	}
	return nil
}

// adaptersGen reports the exported constants, variables and types of the declaration g of an adapters package that are
// not structs, and the types named as a name reserved to the use cases.
func adaptersGen(m *module, f *goFile, g *ast.GenDecl) []string {
	var out []string
	for _, v := range valueSpecs(g) {
		for _, id := range v.Names {
			switch {
			case reserved(id.Name):
				out = append(out, reservedAdapters(m, f, id))
			case id.IsExported():
				out = append(out, fmt.Sprintf("%s: declares the exported %s %s: an adapters package declares only exported "+
					"structs, with their methods", m.pos(f, id), valueKind(g.Tok), id.Name))
			}
		}
	}
	for _, t := range typeSpecs(g) {
		_, isStruct := t.Type.(*ast.StructType)
		switch {
		case reserved(t.Name.Name):
			out = append(out, reservedAdapters(m, f, t.Name))
		case t.Name.IsExported() && !isStruct:
			out = append(out, fmt.Sprintf("%s: declares the exported %s %s: an adapters package declares only exported "+
				"structs, with their methods", m.pos(f, t.Name), typeKind(t.Type), t.Name.Name))
		}
	}
	return out
}

// reservedAdapters is the violation of a name of an adapters package that is reserved to the use cases.
func reservedAdapters(m *module, f *goFile, id *ast.Ident) string {
	return fmt.Sprintf("%s: declares %s, a name reserved to the use cases (GOARCH-23): an adapters package is no use case",
		m.pos(f, id), id.Name)
}

// valueKind names the kind of the values a declaration of tok declares: constant or variable.
func valueKind(tok token.Token) string {
	if tok == token.CONST {
		return "constant"
	}
	return "variable"
}

// typeKind names the kind of a type that is not a struct: interface, function type or type.
func typeKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.InterfaceType:
		return "interface"
	case *ast.FuncType:
		return "function type"
	}
	return "type"
}

// receiverName returns the name of the type of the receiver of the method d, without its pointer.
func receiverName(d *ast.FuncDecl) string {
	if d.Recv == nil || len(d.Recv.List) == 0 {
		return ""
	}
	t := d.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
