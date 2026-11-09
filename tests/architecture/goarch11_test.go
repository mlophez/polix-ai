package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-11: the names of a use case are fixed, the same in every use case package: logic.go declares the logic
// Decide and its Output; in.go declares the entry Command, Request, Response and Port; out.go declares Adapter.
// Outside its package they read with the alias <noun><verb> of the use case (rulecheck.Command).
func TestGOARCH11(t *testing.T) {
	checkRule(t, checkNames, "internal/app/order/place/in.go: declares no type Request",
		"internal/app/order/place/in.go: declares no type Response",
		"internal/app/order/ship/out.go: declares no type Adapter",
		"internal/app/order/ship/logic.go: declares no func Decide")
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
	}
	return out
}
