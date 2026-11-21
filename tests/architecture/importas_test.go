package architecture

import (
	"strings"
	"testing"
)

// TestImportas checks the patterns of importas in .golangci.yml: a use case is imported as <noun><verb> and an adapters
// package as <noun>adapters, and an import without its alias is reported. importas alone runs on the module under
// check, where it finds nothing, and on testdata/depguard, where it reports the imports of a use case and of an
// adapters package without alias, and none of the aliased ones.
func TestImportas(t *testing.T) {
	checkLint(t, depguardRoot, func(t *testing.T, at string) []string { return mustLint(t, at, "importas") },
		"cmd/order/aliases.go:6: import \"github.com/mlophez/polix-ai/internal/app/order/features/place\" imported "+
			"without alias but must be with alias \"orderplace\"",
		"cmd/order/aliases.go:5: import \"github.com/mlophez/polix-ai/internal/app/order/adapters\" imported without "+
			"alias but must be with alias \"orderadapters\"")
	t.Run("accepts", func(t *testing.T) {
		for _, f := range mustLint(t, depguardRoot, "importas") {
			if strings.HasPrefix(f, "cmd/order/aliased.go") {
				t.Errorf("an aliased import is reported: %s", f)
			}
		}
	})
}
