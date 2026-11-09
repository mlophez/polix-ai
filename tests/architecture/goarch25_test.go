package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"
)

// GOARCH-25: an exported struct of internal/app/ has only exported fields: the structs that go from one part to
// another (domain, Request, Response, Output, Adapter) are open to read. Unexported structs, details of one file,
// are free.
func TestGOARCH25(t *testing.T) {
	checkRule(t, checkExportedFields, "internal/app/order/order_line.go:6: Line has the unexported field price")
}

func checkExportedFields(m *module) []string {
	var out []string
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, s := range g.Specs {
				t := s.(*ast.TypeSpec)
				st, ok := t.Type.(*ast.StructType)
				if !ok || !t.Name.IsExported() {
					continue
				}
				for _, field := range st.Fields.List {
					for _, id := range fieldNames(field) {
						if !id.IsExported() {
							out = append(out, fmt.Sprintf("%s: %s has the unexported field %s", m.pos(f, id), t.Name.Name,
								id.Name))
						}
					}
				}
			}
		}
	}
	return out
}

// fieldNames returns the names of a field of a struct; an embedded field is named after its type.
func fieldNames(field *ast.Field) []*ast.Ident {
	if len(field.Names) > 0 {
		return field.Names
	}
	t := field.Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch t := t.(type) {
	case *ast.Ident:
		return []*ast.Ident{t}
	case *ast.SelectorExpr:
		return []*ast.Ident{t.Sel}
	}
	return nil
}
