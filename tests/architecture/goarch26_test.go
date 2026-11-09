package architecture

import (
	"fmt"
	"go/ast"
	"go/types"
	"sort"
	"strings"
	"testing"
)

// GOARCH-26: every exported struct of the domain has the method Valid() error, with a value receiver, which tells
// why a value is not valid. Go declares it in the package of the struct, so in its domain package.
func TestGOARCH26(t *testing.T) {
	checkRule(t, checkValid, "internal/app/order: Note has no method Valid() error",
		"internal/app/order: Item has Valid with a pointer receiver, not Valid() error on a value",
		"internal/app/order: Pack has Valid() bool, not Valid() error")
}

func checkValid(m *module) []string {
	have, wrong := map[string]bool{}, map[string]string{}
	for _, f := range m.sources() {
		for _, d := range f.ast.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Valid" || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			key := f.dir + "." + receiver(fn)
			_, isValue := fn.Recv.List[0].Type.(*ast.Ident)
			res := fn.Type.Results
			switch {
			case !isValue:
				wrong[key] = "Valid with a pointer receiver, not Valid() error on a value"
			case len(fn.Type.Params.List) > 0 || res == nil || len(res.List) != 1 || len(res.List[0].Names) > 1 ||
				types.ExprString(res.List[0].Type) != "error":
				wrong[key] = "Valid" + strings.TrimPrefix(types.ExprString(fn.Type), "func") + ", not Valid() error"
			default:
				have[key] = true
			}
		}
	}
	var out []string
	for key := range m.domainStructs() {
		if have[key] {
			continue
		}
		dir, name, _ := strings.Cut(key, ".")
		if why, ok := wrong[key]; ok {
			out = append(out, fmt.Sprintf("%s: %s has %s", dir, name, why))
			continue
		}
		out = append(out, fmt.Sprintf("%s: %s has no method Valid() error", dir, name))
	}
	sort.Strings(out)
	return out
}
