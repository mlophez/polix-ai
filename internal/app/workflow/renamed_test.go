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

// CUJ-04, AC-03: in an item of check_on_save, a name of on that git can never give its hooks is a name this polix
// knows, never an option of a newer polix: a placeholder of an agent's event or of a commit already made
// (AgentOnlyBuiltins), or a key of check_on_stop. A measure git can compute may be one, and so may every unknown
// placeholder of the entries written in githooks, as before check_on_save. A renamed name is known too, and a name
// polix knows nowhere is not.
func TestProblemKnown(t *testing.T) {
	save := func(name string) string {
		return "polix.yaml:4: guardrails.check_on_save: unknown placeholder {" + name +
			"} (githooks allow {branch}, or a variable of vars)"
	}
	githooks := func(ev, name string) string {
		return "polix.yaml:4: githooks." + ev + ": unknown placeholder {" + name +
			"} (githooks allow {branch}, or a variable of vars)"
	}
	for p, want := range map[string]bool{
		save("path"):    true,
		save("command"): true,
		save("commit"):  true,
		save("subject"): true,
		`polix.yaml:4: unknown key "wait_agents" in guardrails.check_on_save (run, changed, message)`: true,
		`polix.yaml:3: unknown key "run_on_stop" in guardrails (protected_files): use check_on_stop`:  true,
		save("changed_lines"):                   false,
		save("changed_files"):                   false,
		save("risk_score"):                      false,
		save("branch2"):                         false,
		save("kye"):                             false,
		githooks("pre_commit", "command"):       false,
		githooks("pre_commit", "path"):          false,
		githooks("pre_commit", "changed_files"): false,
		githooks("pre_commit", "risk_score"):    false,
		githooks("commit_msg", "commit"):        false,
		"polix.yaml:4: unknown placeholder {path2} (built in: path, command; or a variable of vars)": false,
		`polix.yaml:4: unknown key "retries" in guardrails.check_on_save (run, changed, message)`:    false,
		`polix.yaml:4: unknown key "always" in guardrails.exec_on_start (run, message)`:              false,
		`polix.yaml:4: unknown key "x"`: false,
	} {
		if got := Problem(p).Known(); got != want {
			t.Errorf("Known(%s) = %v, want %v", p, got, want)
		}
	}
}
