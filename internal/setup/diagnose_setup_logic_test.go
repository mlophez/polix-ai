package setup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// healthy is the inspection of a ready machine and repository: the latest release, on the PATH, a valid
// workflow, the example settings, the plugin installed and Claude Code with it.
func healthy(t *testing.T) Inspection {
	t.Helper()
	s := Setup{Home: "/h", Root: "/r", Agents: []string{Claude, Codex}, Version: "0.2.0"}
	settings, err := os.ReadFile(filepath.Join("..", "..", "examples", "claude-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return Inspection{Setup: s, Exe: "/bin/polix", Latest: "0.2.0", Path: "/bin/polix", SamePath: true, Found: true,
		Files: map[string][]byte{s.HooksFile(Claude): settings,
			s.PluginManifest(): []byte(`{"name": "polix", "version": "0.2.0"}`)},
		Embedded: logicPlugin, Claude: []byte(`[{"id":"polix@polix","scope":"user","enabled":true}]`),
		StateDir: "/r/state"}
}

// text is the report as lines "level area message".
func text(out DiagnoseSetupOutput) string {
	var b strings.Builder
	for _, f := range out.Findings {
		b.WriteString(f.Level + " " + f.Area + " " + f.Message + "\n")
	}
	return b.String()
}

func TestDiagnoseSetupHealthy(t *testing.T) {
	out := DiagnoseSetup(healthy(t), false)
	if out.Failed || strings.Contains(text(out), "warn") || len(out.Findings) != 8 {
		t.Fatalf("healthy:\n%s", text(out))
	}
}

func TestDiagnoseSetupFindsProblems(t *testing.T) {
	i := healthy(t)
	var settings map[string]map[string][]map[string]any
	if err := json.Unmarshal(i.Files[i.Setup.HooksFile(Claude)], &settings); err != nil {
		t.Fatal(err)
	}
	delete(settings["hooks"], "Stop")
	settings["hooks"]["PreToolUse"][0]["matcher"] = "Edit|Write"
	settings["hooks"]["PostToolUse"][0]["hooks"] = []map[string]any{{"type": "command", "command": "polix hook claude"}}
	data, _ := json.Marshal(settings)
	i.Files[i.Setup.HooksFile(Claude)] = data
	i.Latest = "0.3.0"
	i.WorkflowErr = workflow.Invalid{`polix.yaml:5: unknown key "foo"`}
	i.Rules = []rule.Rule{{ID: "A-1", Description: "d", Deterministic: true, Check: "checks/a.sh", File: "ai/rules/a.yaml"}}
	i.Runnable = map[string]error{"checks/a.sh": errors.New("check checks/a.sh is not executable (chmod +x)")}
	out := DiagnoseSetup(i, false)
	got := text(out)
	for _, w := range []string{"the latest release is 0.3.0", `unknown key "foo"`, "newer polix",
		"Stop does not run polix hook claude", "the matcher of PreToolUse does not cover MultiEdit, NotebookEdit, Bash",
		"PostToolUse: the timeout", "check checks/a.sh is not executable"} {
		if !strings.Contains(got, w) {
			t.Errorf("doctor should report %q:\n%s", w, got)
		}
	}
	if !out.Failed {
		t.Fatal("it fails")
	}
	// A check that a runner of rules.run runs is never reported as not executable.
	i.WorkflowErr = nil
	i.Workflow = workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{{Glob: "*.sh", Argv: []string{"sh", "{file}"}}}}}
	if got := text(DiagnoseSetup(i, false)); strings.Contains(got, "not executable") {
		t.Fatalf("a check of a runner:\n%s", got)
	}
}

func TestDiagnoseSetupHooks(t *testing.T) {
	i := healthy(t)
	file := i.Setup.HooksFile(Claude)
	settings := string(i.Files[file])
	i.Files[file] = []byte(strings.Replace(settings, `"polix update --hook"`, `"polix update --hook; polix doctor --hook"`, 1))
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "command of an earlier polix") {
		t.Fatalf("legacy command:\n%s", got)
	}
	i.Files[file] = []byte(strings.Replace(settings, `"polix update --hook"`, `"echo hi"`, 1))
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "does not update itself") {
		t.Fatalf("no update command:\n%s", got)
	}
	var doc map[string]map[string]any
	_ = json.Unmarshal([]byte(settings), &doc)
	delete(doc["hooks"], "PostToolUseFailure")
	i.Files[file], _ = json.Marshal(doc)
	if out := DiagnoseSetup(i, false); out.Failed || !strings.Contains(text(out), "warn hooks PostToolUseFailure does not run") {
		t.Fatalf("old settings only warn:\n%s", text(out))
	}
	i.Files[filepath.FromSlash("/h/.claude/settings.json")] = []byte(settings)
	i.Files[file] = []byte(settings)
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "runs polix twice") {
		t.Fatalf("duplicated:\n%s", got)
	}
	i.Files = map[string][]byte{}
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "does not run polix in this repository") {
		t.Fatalf("no hooks:\n%s", got)
	}
}

func TestDiagnoseSetupMachine(t *testing.T) {
	i := healthy(t)
	i.PathErr = errors.New("not found")
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "fail path polix is not on the PATH") {
		t.Fatal(got)
	}
	i.PathErr, i.SamePath = nil, false
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "the PATH runs another polix") {
		t.Fatal(got)
	}
	i.RepoErr = errors.New("not inside")
	if out := DiagnoseSetup(i, false); len(out.Findings) != 4 || !strings.Contains(text(out), "ok repo not inside a git repository") {
		t.Fatalf("outside a repository:\n%s", text(out))
	}
	i.GitErr = errors.New("no git")
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "fail git git is not on the PATH") {
		t.Fatal(got)
	}
	i = healthy(t)
	i.Setup.Version = "dev"
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "development build") || !strings.Contains(got, "is from polix 0.2.0") {
		t.Fatal(got)
	}
	i = healthy(t)
	i.Claude, i.ClaudeErr = nil, ErrNoCLI
	if out := DiagnoseSetup(i, false); out.Failed || strings.Contains(text(out), "warn") {
		t.Fatalf("without Claude Code:\n%s", text(out))
	}
	i.Claude, i.ClaudeErr = []byte("[]"), nil
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "does not have polix@polix installed") {
		t.Fatal(got)
	}
	if got := text(DiagnoseSetup(i, true)); strings.Contains(got, "does not have") {
		t.Fatal("the hook does not ask Claude Code")
	}
	i.CodexUsed = true
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "not enabled in Codex") || !strings.Contains(got, "Codex is used here") {
		t.Fatal(got)
	}
	i.StateErr = errors.New("read-only")
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "cannot write the state in /r/state: read-only") {
		t.Fatal(got)
	}
}

// doctor warns that rules check --changed would fail when no base branch exists and a rule has a check.
func TestDiagnoseSetupMissingBase(t *testing.T) {
	i := healthy(t)
	i.BaseErr = errors.New("no main")
	if got := text(DiagnoseSetup(i, false)); strings.Contains(got, "would fail") {
		t.Fatal("no deterministic rule, no warning")
	}
	i.Rules = []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "c.sh", File: "f"}}
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "rules check --changed would fail") {
		t.Fatal(got)
	}
	i.Found = false
	if got := text(DiagnoseSetup(i, false)); !strings.Contains(got, "no polix.yaml") || !strings.Contains(got, "would fail") {
		t.Fatal(got)
	}
}
