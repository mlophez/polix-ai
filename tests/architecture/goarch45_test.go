package architecture

import (
	"fmt"
	"testing"
)

// maxDecideParams is the most parameters the logic Decide of a use case takes (GOARCH-45).
const maxDecideParams = 4

// GOARCH-45: the logic Decide of a use case takes at most 4 parameters, each name counted (a, b string are two);
// what does not fit is grouped into a value of the domain.
func TestGOARCH45(t *testing.T) {
	checkRule(t, checkDecideParams,
		"internal/app/stock/drop/logic.go:6: Decide takes 5 parameters, more than 4: group them into a value of the "+
			"domain")
	checkAccepts(t, checkDecideParams, "internal/app/stock/take/", "internal/app/stock/load/")
}

func checkDecideParams(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		logic := uc.files[kindLogic]
		if logic == nil {
			continue // GOARCH-3
		}
		fn := funcDecl(logic.ast, "Decide")
		if fn == nil {
			continue // GOARCH-11
		}
		n := 0
		for _, p := range fn.Type.Params.List {
			n += max(len(p.Names), 1)
		}
		if n > maxDecideParams {
			out = append(out, fmt.Sprintf("%s: Decide takes %d parameters, more than %d: group them into a value of "+
				"the domain", m.pos(logic, fn), n, maxDecideParams))
		}
	}
	return out
}
