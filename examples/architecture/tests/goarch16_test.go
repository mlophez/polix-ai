package tests

import (
	"fmt"
	"go/ast"
	"go/token"
	"testing"
)

// GOARCH-16, the part a check can decide: every use case declares <Verb><Noun>Request and <Verb><Noun>Response in
// its entry, and neither the types of its logic nor the exported types of the domain carry tags, the shape of a
// transport. That the response is built from the output is GOARCH-22.
func TestGOARCH16RequestResponse(t *testing.T) {
	checkRule(t, checkRequestResponse, "place_order_in.go: declares no type PlaceOrderResponse",
		"field ID carries the tag `json:\"id\"`", "order_line.go:5: field SKU carries the tag `json:\"sku\"`")
}

func checkRequestResponse(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		if in := uc.files[kindIn]; in != nil {
			have := decls(in.ast)
			for _, name := range []string{camel(uc.stem) + "Request", camel(uc.stem) + "Response"} {
				if have[name] != "type" {
					out = append(out, fmt.Sprintf("%s: declares no type %s", in.rel, name))
				}
			}
		}
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
				for _, s := range g.Specs {
					if t := s.(*ast.TypeSpec); t.Name.IsExported() {
						out = append(out, tags(m, f, t)...)
					}
				}
			}
		}
	}
	return out
}

// tags returns the fields with a tag under n, a node of f.
func tags(m *module, f *goFile, n ast.Node) []string {
	var out []string
	ast.Inspect(n, func(n ast.Node) bool {
		field, ok := n.(*ast.Field)
		if ok && field.Tag != nil && len(field.Names) > 0 {
			out = append(out, fmt.Sprintf("%s: field %s carries the tag %s: no transport shape out of the entry",
				m.pos(f, field), field.Names[0].Name, field.Tag.Value))
		}
		return true
	})
	return out
}
