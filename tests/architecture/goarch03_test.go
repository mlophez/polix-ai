package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-3: a use case folder internal/app/<noun>/<verb>/ has its three files in.go, logic.go and out.go.
func TestGOARCH3(t *testing.T) {
	checkRule(t, checkUseCaseFiles, "internal/app/order/cancel: use case CancelOrder has no logic.go",
		"internal/app/stock/drop: use case DropStock has no in.go")
}

func checkUseCaseFiles(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		for _, f := range useCaseFiles {
			if uc.files[f.kind] == nil {
				out = append(out, fmt.Sprintf("%s: use case %s has no %s", uc.dir, uc.name(), f.name))
			}
		}
	}
	return out
}
