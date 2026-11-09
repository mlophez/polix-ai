package architecture

import (
	"fmt"
	"go/ast"
	"testing"
)

// loadVerb is the verb of a load use case, internal/app/<noun>/load: the loader of a file that several use cases
// read, which cmd/ builds once and injects into them.
const loadVerb = "load"

// GOARCH-11: the names of a use case are fixed, the same in every use case package: logic.go declares the logic
// Decide and its Output; in.go declares the entry Command, Request, Response and Port; out.go declares Adapter.
// Outside its package they read with the alias <noun><verb> of the use case (rulecheck.Command). The domain of a
// load use case declares its loader Load, a named function type, and its result Loaded, a struct.
func TestGOARCH11(t *testing.T) {
	checkRule(t, checkNames, "internal/app/order/place/in.go: declares no type Request",
		"internal/app/order/place/in.go: declares no type Response",
		"internal/app/order/ship/out.go: declares no type Adapter",
		"internal/app/order/ship/logic.go: declares no func Decide",
		"internal/app/order/load: the domain order declares no func type Load, the loader of its load use case",
		"internal/app/order/load: the domain order declares no struct Loaded, the result of its loader")
	checkAccepts(t, checkNames, "internal/app/stock/load")
}

func checkNames(m *module) []string {
	want := map[kind][][2]string{
		kindLogic: {{"func", "Decide"}, {"type", "Output"}},
		kindIn:    {{"func", "Command"}, {"type", "Request"}, {"type", "Response"}, {"type", "Port"}},
		kindOut:   {{"type", "Adapter"}},
	}
	var out []string
	for _, uc := range m.useCases() {
		for _, file := range useCaseFiles {
			f := uc.files[file.kind]
			if f == nil {
				continue // GOARCH-3
			}
			have := decls(f.ast)
			for _, w := range want[file.kind] {
				if have[w[1]] != w[0] {
					out = append(out, fmt.Sprintf("%s: declares no %s %s", f.rel, w[0], w[1]))
				}
			}
		}
		if uc.verb == loadVerb {
			out = append(out, loaderTypes(m, uc)...)
		}
	}
	return out
}

// loaderTypes reports the types the domain of the load use case uc lacks: the loader Load, a named function type,
// and its result Loaded, a struct.
func loaderTypes(m *module, uc *useCase) []string {
	domain := "internal/app/" + uc.noun
	var load, loaded bool
	for _, f := range m.packages()[domain] {
		if t := typeSpec(f.ast, "Load"); t != nil {
			_, load = t.Type.(*ast.FuncType)
		}
		if t := typeSpec(f.ast, "Loaded"); t != nil {
			_, loaded = t.Type.(*ast.StructType)
		}
	}
	var out []string
	if !load {
		out = append(out, fmt.Sprintf("%s: the domain %s declares no func type Load, the loader of its load use "+
			"case", uc.dir, uc.noun))
	}
	if !loaded {
		out = append(out, fmt.Sprintf("%s: the domain %s declares no struct Loaded, the result of its loader", uc.dir,
			uc.noun))
	}
	return out
}
