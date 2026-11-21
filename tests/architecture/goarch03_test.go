package architecture

import (
	"fmt"
	"testing"
)

// GOARCH-3: a use case folder internal/app/<noun>/features/<verb>/ has its three files <verb>_in.go,
// <verb>_logic.go and <verb>_out.go; their siblings <verb>_in_<topic>.go, <verb>_logic_<topic>.go and
// <verb>_out_<topic>.go are optional and never stand for their main file.
func TestGOARCH3(t *testing.T) {
	checkRule(t, checkUseCaseFiles, "internal/app/order/features/cancel: use case CancelOrder has no cancel_logic.go",
		"internal/app/stock/features/drop: use case DropStock has no drop_in.go")
	checkAccepts(t, checkUseCaseFiles, "internal/app/stock/features/take:")
}

func checkUseCaseFiles(m *module) []string {
	var out []string
	for _, uc := range m.useCases() {
		for _, f := range useCaseFiles {
			if uc.files[f.of] == nil { // a sibling alone is no main file
				out = append(out, fmt.Sprintf("%s: use case %s has no %s", uc.dir, uc.name(), uc.file(f.kind)))
			}
		}
	}
	return out
}
