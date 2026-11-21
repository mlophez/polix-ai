package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-11: the names of a use case are fixed, the same in every use case package: <verb>_logic.go declares the
// logic Decide and its Output; <verb>_in.go declares the entry Command, Request, Response and Port; <verb>_out.go
// declares Adapter.
// Outside its package they read with the alias <noun><verb> of the use case (rulecheck.Command).
func TestGOARCH11(t *testing.T) {
	checkRule(t, checkNames, "internal/app/order/features/place/place_in.go: declares no type Request",
		"internal/app/order/features/place/place_in.go: declares no type Response",
		"internal/app/order/features/ship/ship_out.go: declares no type Adapter",
		"internal/app/order/features/ship/ship_logic.go: declares no func Decide")
	checkAccepts(t, checkNames, "internal/app/stock/features/take")
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
			f := uc.files[file.of]
			if f == nil {
				continue // GOARCH-3
			}
			have := decls(f.ast)
			for _, w := range want[file.of] {
				if have[w[1]] != w[0] {
					out = append(out, fmt.Sprintf("%s: declares no %s %s", f.rel, w[0], w[1]))
				}
			}
		}
	}
	return out
}
