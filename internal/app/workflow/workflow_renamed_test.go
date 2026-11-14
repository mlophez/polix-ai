package workflow

import (
	"strings"
	"testing"
)

// AC-04, AC-05: the problems of the removed folder keys, plans and reports at top level and path in rules, carry
// their hint, so they are names this polix replaced, never options of a newer polix.
func TestProblemRenamed(t *testing.T) {
	plans := UnknownKeyError{Key: "plans", Line: 2}.Error()
	reports := UnknownKeyError{Key: "reports", Line: 3}.Error()
	if plans != `polix.yaml:2: unknown key "plans": remove it: the plans go to plans/ below path (ai/plans by default)` ||
		reports != `polix.yaml:3: unknown key "reports": remove it: the reports go to reports/ below path (ai/reports by default)` {
		t.Fatalf("hints: %s\n%s", plans, reports)
	}
	rulesPath := strings.Join(Workflow{Version: 1, Rules: Rules{Keys: []string{"path", "run"}}}.Problems(), "\n")
	if rulesPath != `polix.yaml: unknown key "path" in rules (run): move it to the top-level key path, the folder that holds rules/ (ai by default)` {
		t.Fatalf("rules.path: %s", rulesPath)
	}
	for _, p := range []string{plans, reports, rulesPath} {
		if !Problem(p).Renamed() {
			t.Errorf("not renamed: %s", p)
		}
	}
	for _, p := range []string{
		`polix.yaml: unknown key "folder" in rules (run)`,
		`polix.yaml:2: unknown key "plan"`,
		`polix.yaml: unknown key "dir" in rules (run): remove it`,
	} {
		if Problem(p).Renamed() {
			t.Errorf("renamed: %s", p)
		}
	}
}
