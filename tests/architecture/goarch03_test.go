package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-3: a use case folder internal/app/<noun>/<verb>/ has its three files in.go, logic.go and out.go; their
// siblings in_<topic>.go, logic_<topic>.go and out_<topic>.go are optional and never stand for their main file.
func TestGOARCH3(t *testing.T) {
	checkRule(t, checkUseCaseFiles, "internal/app/order/cancel: use case CancelOrder has no logic.go",
		"internal/app/stock/drop: use case DropStock has no in.go")
	checkAccepts(t, checkUseCaseFiles, "internal/app/stock/take:")
}

func checkUseCaseFiles(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		for _, f := range useCaseFiles {
			if uc.files[f.kind] == nil { // a sibling alone is no main file
				out = append(out, fmt.Sprintf("%s: use case %s has no %s", uc.dir, uc.name(), f.name))
			}
		}
	}
	return out
}
