package architecture

import (
	"strings"
	"testing"
)

// limitsRoot is a module whose Go file breaks each limit of GO-4 and GO-5 once and whose test file breaks them too, so
// TestGO4 and TestGO5 prove that golangci-lint detects each limit and leaves the tests out. Like testdata/depguard it
// compiles, since golangci-lint loads its types.
const limitsRoot = "testdata/limits"

// limitLinters are the linters that decide GO-4 and GO-5: gocyclo, gocognit and revive, of which lint keeps only the rule
// file-length-limit.
var limitLinters = []string{"gocyclo", "gocognit", "revive"}

// GO-4: a function that is not in a _test.go file has a cyclomatic complexity of at most 15 and a cognitive
// complexity of at most 20, as gocyclo and gocognit decide with the .golangci.yml of the module. The test files are
// left out.
func TestGO4(t *testing.T) {
	checkLint(t, limitsRoot, limitFindings("cyclomatic complexity", "cognitive complexity"),
		"limits.go:6: cyclomatic complexity 16 of func `Branchy` is high (> 15)",
		"limits.go:44: cognitive complexity 21 of func `Nested` is high (> 20)")
}

// GO-5: a Go file that is not a _test.go has at most 400 lines, as the rule file-length-limit of revive decides with
// the .golangci.yml of the module. The test files are left out.
func TestGO5(t *testing.T) {
	checkLint(t, limitsRoot, limitFindings("file-length-limit"),
		"limits.go:460: file-length-limit: file length is 460 lines, which exceeds the limit of 400")
}

// limitFindings runs the linters of the limits once per module and keeps the findings that mention one of marks, so
// GO-4 and GO-5 each see only their own limits.
func limitFindings(marks ...string) func(t *testing.T, at string) []string {
	return func(t *testing.T, at string) []string {
		var out []string
		for _, f := range mustLint(t, at, limitLinters...) {
			for _, m := range marks {
				if strings.Contains(f, m) {
					out = append(out, f)
					break
				}
			}
		}
		return out
	}
}
