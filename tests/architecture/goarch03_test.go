package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-3: a use case is three files, <verb>_<noun>_in.go, <verb>_<noun>_logic.go and <verb>_<noun>_out.go.
func TestGOARCH3(t *testing.T) {
	checkRule(t, checkUseCaseFiles, "use case cancel_order has no cancel_order_logic.go",
		"use case drop_stock has no drop_stock_in.go")
}

func checkUseCaseFiles(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		for _, s := range useCaseSuffixes {
			if uc.files[s.kind] == nil {
				out = append(out, fmt.Sprintf("%s: use case %s has no %s%s.go", uc.dir, uc.stem, uc.stem, s.suffix))
			}
		}
	}
	return out
}
