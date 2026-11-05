package tests

import (
	"fmt"
	"go/ast"
	"testing"
)

// GOARCH-16, the part a check can decide: every use case declares <Verb><Noun>Request and <Verb><Noun>Response in
// its entry, and the types of its logic carry no tags, the shape of a transport. That the response is built from
// the output is GOARCH-22.
func TestGOARCH16RequestResponse(t *testing.T) {
	checkRule(t, checkRequestResponse, "place_order_in.go: declares no type PlaceOrderResponse",
		"field ID carries the tag `json:\"id\"`")
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
			ast.Inspect(logic.ast, func(n ast.Node) bool {
				f, ok := n.(*ast.Field)
				if ok && f.Tag != nil && len(f.Names) > 0 {
					out = append(out, fmt.Sprintf("%s: field %s carries the tag %s: the logic has no transport shape",
						m.pos(logic, f), f.Names[0].Name, f.Tag.Value))
				}
				return true
			})
		}
	}
	return out
}
