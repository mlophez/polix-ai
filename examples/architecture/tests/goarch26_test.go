package tests

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"
)

// GOARCH-26: every exported struct of the domain has the method Valid() error, with a value receiver, which tells
// why a value is not valid.
func TestGOARCH26Valid(t *testing.T) {
	checkRule(t, checkValid, "internal/order: Note has no method Valid() error")
}

func checkValid(m *module) []string {
	have := map[string]bool{}
	for _, f := range m.sources() {
		if f.kind != kindDomain {
			continue
		}
		for _, d := range f.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Valid" || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			recv, isValue := fn.Recv.List[0].Type.(*ast.Ident)
			res := fn.Type.Results
			if isValue && len(fn.Type.Params.List) == 0 && res != nil && len(res.List) == 1 &&
				len(res.List[0].Names) <= 1 && types.ExprString(res.List[0].Type) == "error" {
				have[f.dir+"."+recv.Name] = true
			}
		}
	}
	var out []string
	for key := range m.domainStructs() {
		if !have[key] {
			dir, name, _ := strings.Cut(key, ".")
			out = append(out, fmt.Sprintf("%s: %s has no method Valid() error", dir, name))
		}
	}
	sort.Strings(out)
	return out
}
