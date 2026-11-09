package architecture

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"
)

// GOARCH-16, the part a check can decide: neither the types of the logic nor the exported types of the domain
// carry field tags, the shape of a transport, on named or embedded fields. That the use case declares its
// Request and Response is GOARCH-11; that the response is built from the output is GOARCH-22.
func TestGOARCH16(t *testing.T) {
	checkRule(t, checkRequestResponse,
		"place/logic.go:19: field ID carries the tag `json:\"id\"`",
		"internal/app/order/order_line.go:5: field SKU carries the tag `json:\"sku\"`",
		"field Item carries the tag `json:\"item\"`")
}

func checkRequestResponse(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		if logic := uc.files[kindLogic]; logic != nil {
			out = append(out, tags(m, logic, logic.ast)...)
		}
	}
	for _, f := range m.sources() {
		if f.kind != kindDomain {
			continue
		}
		for _, d := range f.ast.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.TYPE {
				for _, t := range typeSpecs(g) {
					if t.Name.IsExported() {
						out = append(out, tags(m, f, t)...)
					}
				}
			}
		}
	}
	return out
}

// tags returns the fields with a tag under n, a node of f; an embedded field is named after its type.
func tags(m *module, f *goFile, n ast.Node) []string {
	var out []string
	ast.Inspect(n, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if ok && field.Tag != nil {
			for _, id := range fieldNames(field) {
				out = append(out, fmt.Sprintf("%s: field %s carries the tag %s: no transport shape out of the entry",
					m.pos(f, field), id.Name, field.Tag.Value))
			}
		}
		return true
	})
	return out
}
