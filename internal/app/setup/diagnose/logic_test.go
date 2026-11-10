package diagnose

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

var logicPlugin = map[string][]byte{
	"polix/.claude-plugin/plugin.json": []byte(`{"name": "polix", "version": "0", "description": "d"}`),
	"polix/skills/plan/SKILL.md":       []byte("x"),
	"polix/agents/reviewer.md":         []byte("x"),
}

// hooksOf is a hooks file with the content data, as the adapter reads it: the handlers that run a command, in
// the groups that keep one; a content that is not JSON is its error.
func hooksOf(data []byte) setup.HooksFile {
	var doc struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string  `json:"command"`
				Timeout float64 `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return setup.HooksFile{Err: err}
	}
	hooks := map[string][]setup.HookGroup{}
	for ev, groups := range doc.Hooks {
		for _, g := range groups {
			group := setup.HookGroup{Matcher: g.Matcher}
			for _, h := range g.Hooks {
				if h.Command != "" {
					group.Handlers = append(group.Handlers, setup.HookHandler{Command: h.Command, Timeout: h.Timeout})
				}
			}
			if len(group.Handlers) > 0 {
				hooks[ev] = append(hooks[ev], group)
			}
		}
	}
	return setup.HooksFile{Hooks: hooks}
}

// exampleSettings is the example settings of Claude Code of the repository, what install writes.
func exampleSettings(t *testing.T) []byte {
	t.Helper()
	settings, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "examples", "claude-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return settings
}

// healthy is the inspection of a ready machine and repository: the latest release, on the PATH, a valid
// workflow, the example settings, the plugin installed and Claude Code with it.
func healthy(t *testing.T) setup.Inspection {
	t.Helper()
	s := setup.Setup{Home: "/h", Root: "/r", Agents: []string{setup.Claude, setup.Codex}, Version: "0.2.0"}
	return setup.Inspection{Setup: s, Exe: "/bin/polix", Latest: "0.2.0", Path: "/bin/polix", SamePath: true, Found: true,
		ClaudeHooks:     map[string]setup.HooksFile{s.HooksFile(setup.Claude): hooksOf(exampleSettings(t))},
		Plugin:          setup.Plugin{Written: true, Manifest: "0.2.0", Embedded: logicPlugin, Version: "0.2.0"},
		ClaudeInstalled: true, ClaudeEnabled: true, StateDir: "/r/state"}
}

// text is the report as lines "level area message".
func text(out Output) string {
	var b strings.Builder
	for _, f := range out.Findings {
		b.WriteString(f.Level + " " + f.Area + " " + f.Message + "\n")
	}
	return b.String()
}

func TestDecideHealthy(t *testing.T) {
	out := Decide(healthy(t), false)
	if out.Failed || strings.Contains(text(out), "warn") || len(out.Findings) != 8 {
		t.Fatalf("healthy:\n%s", text(out))
	}
}

func TestDecideFindsProblems(t *testing.T) {
	i := healthy(t)
	var settings map[string]map[string][]map[string]any
	if err := json.Unmarshal(exampleSettings(t), &settings); err != nil {
		t.Fatal(err)
	}
	delete(settings["hooks"], "Stop")
	settings["hooks"]["PreToolUse"][0]["matcher"] = "Edit|Write"
	settings["hooks"]["PostToolUse"][0]["hooks"] = []map[string]any{{"type": "command", "command": "polix hook claude"}}
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	i.ClaudeHooks[i.Setup.HooksFile(setup.Claude)] = hooksOf(data)
	i.Latest = "0.3.0"
	i.WorkflowErr = workflow.InvalidError{`polix.yaml:5: unknown key "foo"`}
	i.Rules = []rule.Rule{{ID: "A-1", Description: "d", Deterministic: true, Check: "checks/a.sh", File: "ai/rules/a.yaml"}}
	i.Runnable = map[string]error{"checks/a.sh": errors.New("check checks/a.sh is not executable (chmod +x)")}
	out := Decide(i, false)
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
	if got := text(Decide(i, false)); strings.Contains(got, "not executable") {
		t.Fatalf("a check of a runner:\n%s", got)
	}
}

func TestDecideHooks(t *testing.T) {
	i := healthy(t)
	file := i.Setup.HooksFile(setup.Claude)
	settings := string(exampleSettings(t))
	i.ClaudeHooks[file] = hooksOf([]byte(strings.Replace(settings, `"polix update --hook"`, `"polix update --hook; polix doctor --hook"`, 1)))
	if got := text(Decide(i, false)); !strings.Contains(got, "command of an earlier polix") {
		t.Fatalf("legacy command:\n%s", got)
	}
	i.ClaudeHooks[file] = hooksOf([]byte(strings.Replace(settings, `"polix update --hook"`, `"echo hi"`, 1)))
	if got := text(Decide(i, false)); !strings.Contains(got, "does not update itself") {
		t.Fatalf("no update command:\n%s", got)
	}
	for _, ev := range []string{"PostToolUseFailure", "SubagentStart"} {
		var doc map[string]map[string]any
		_ = json.Unmarshal([]byte(settings), &doc)
		delete(doc["hooks"], ev)
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		i.ClaudeHooks[file] = hooksOf(data)
		out := Decide(i, false)
		if out.Failed || !strings.Contains(text(out), "warn hooks "+ev+" does not run polix hook claude: run polix install") {
			t.Fatalf("old settings without %s only warn:\n%s", ev, text(out))
		}
	}
	i.ClaudeHooks[filepath.FromSlash("/h/.claude/settings.json")] = hooksOf([]byte(settings))
	i.ClaudeHooks[file] = hooksOf([]byte(settings))
	if got := text(Decide(i, false)); !strings.Contains(got, "runs polix twice") {
		t.Fatalf("duplicated:\n%s", got)
	}
	i.ClaudeHooks[filepath.FromSlash("/h/.claude/settings.json")] = setup.HooksFile{ReadErr: errors.New("denied")}
	if got := text(Decide(i, false)); !strings.Contains(got, "fail hooks "+filepath.FromSlash("/h/.claude/settings.json")+": denied") {
		t.Fatalf("a settings file that cannot be read:\n%s", got)
	}
	i.ClaudeHooks = nil
	if got := text(Decide(i, false)); !strings.Contains(got, "does not run polix in this repository") {
		t.Fatalf("no hooks:\n%s", got)
	}
}

func TestDecideMachine(t *testing.T) {
	i := healthy(t)
	i.PathErr = errors.New("not found")
	if got := text(Decide(i, false)); !strings.Contains(got, "fail path polix is not on the PATH") {
		t.Fatal(got)
	}
	i.PathErr, i.SamePath = nil, false
	if got := text(Decide(i, false)); !strings.Contains(got, "the PATH runs another polix") {
		t.Fatal(got)
	}
	i.RepoErr = errors.New("not inside")
	if out := Decide(i, false); len(out.Findings) != 4 || !strings.Contains(text(out), "ok repo not inside a git repository") {
		t.Fatalf("outside a repository:\n%s", text(out))
	}
	i.GitErr = errors.New("no git")
	if got := text(Decide(i, false)); !strings.Contains(got, "fail git git is not on the PATH") {
		t.Fatal(got)
	}
	i = healthy(t)
	i.Setup.Version, i.Plugin.Version = "dev", "dev"
	if got := text(Decide(i, false)); !strings.Contains(got, "development build") || !strings.Contains(got, "is from polix 0.2.0") {
		t.Fatal(got)
	}
	i = healthy(t)
	i.ClaudeInstalled, i.ClaudeEnabled, i.ClaudeErr = false, false, setup.ErrNoCLI
	if out := Decide(i, false); out.Failed || strings.Contains(text(out), "warn") {
		t.Fatalf("without Claude Code:\n%s", text(out))
	}
	i.ClaudeErr = nil
	if got := text(Decide(i, false)); !strings.Contains(got, "does not have polix@polix installed") {
		t.Fatal(got)
	}
	if got := text(Decide(i, true)); strings.Contains(got, "does not have") {
		t.Fatal("the hook does not ask Claude Code")
	}
	i.StateErr = errors.New("read-only")
	if got := text(Decide(i, false)); !strings.Contains(got, "cannot write the state in /r/state: read-only") {
		t.Fatal(got)
	}
}

// codexLines are the lines of the report about Codex.
func codexLines(out Output) string {
	var b strings.Builder
	for _, f := range out.Findings {
		if f.Area == "codex" {
			b.WriteString(f.Level + " " + f.Message + "\n")
		}
	}
	return b.String()
}

// Codex is checked only when polix is registered for it: its hooks run polix or its plugin is enabled.
func TestDecideCodex(t *testing.T) {
	i := healthy(t)
	hooksFile := i.Setup.HooksFile(setup.Codex)
	i.CodexHooks = map[string]setup.HooksFile{}
	if got := codexLines(Decide(i, false)); got != "" {
		t.Fatalf("no Codex files, no Codex finding:\n%s", got)
	}
	// A Codex that was never set up for polix, even with a broken hooks file of its own, is not checked.
	i.CodexHooks[hooksFile] = hooksOf([]byte(`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "echo hi"}]}]}}`))
	if got := codexLines(Decide(i, false)); got != "" {
		t.Fatalf("Codex without polix, no Codex finding:\n%s", got)
	}
	i.CodexHooks[hooksFile] = hooksOf([]byte("{"))
	if got := codexLines(Decide(i, false)); got != "" {
		t.Fatalf("a broken hooks file of a Codex without polix:\n%s", got)
	}
	delete(i.CodexHooks, hooksFile)
	i.CodexEnabled = true
	if got := codexLines(Decide(i, false)); !strings.Contains(got, "warn Codex does not run polix in this repository: run polix install --agent codex") ||
		strings.Contains(got, "ok ") {
		t.Fatalf("the plugin enabled without hooks:\n%s", got)
	}
	i.CodexHooks[hooksFile] = hooksOf([]byte(`{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "polix hook codex"}]}]}}`))
	out := Decide(i, false)
	if got := codexLines(out); got != "ok hooks registered (Codex runs them once trusted with /hooks); "+
		"Codex support is not implemented and untested\n" || out.Failed {
		t.Fatalf("registered and enabled:\n%s", got)
	}
	i.CodexEnabled = false
	if got := codexLines(Decide(i, false)); !strings.Contains(got, "warn the plugin polix@polix is not enabled in Codex") {
		t.Fatalf("registered without the plugin:\n%s", got)
	}
	i.CodexEnabled = true
	i.CodexHooks[hooksFile] = hooksOf([]byte("{"))
	if out := Decide(i, false); !out.Failed || !strings.Contains(codexLines(out), "fail "+hooksFile+": ") {
		t.Fatalf("a hooks file that cannot be parsed fails:\n%s", codexLines(out))
	}
	i.CodexHooks[hooksFile] = setup.HooksFile{ReadErr: errors.New("open " + hooksFile + ": denied")}
	if got := codexLines(Decide(i, false)); !strings.Contains(got, "fail open "+hooksFile+": denied\n") {
		t.Fatalf("a hooks file that cannot be read fails with its error:\n%s", got)
	}
}

// doctor warns that rules check --changed would fail when no base branch exists and a rule has a check.
func TestDecideMissingBase(t *testing.T) {
	i := healthy(t)
	i.BaseErr = errors.New("no main")
	if got := text(Decide(i, false)); strings.Contains(got, "would fail") {
		t.Fatal("no deterministic rule, no warning")
	}
	i.Rules = []rule.Rule{{ID: "S-1", Description: "d", Deterministic: true, Check: "c.sh", File: "f"}}
	if got := text(Decide(i, false)); !strings.Contains(got, "rules check --changed would fail") {
		t.Fatal(got)
	}
	i.Found = false
	if got := text(Decide(i, false)); !strings.Contains(got, "no polix.yaml") || !strings.Contains(got, "would fail") {
		t.Fatal(got)
	}
}

// requirementLines are the lines of the report about the programs the workflow requires.
func requirementLines(out Output) string {
	var b strings.Builder
	for _, f := range out.Findings {
		if f.Area == "requirements" {
			b.WriteString(f.Level + " " + f.Message + "\n")
		}
	}
	return b.String()
}

// A program the PATH does not run is a warning, in the order of the file, right after the workflow, and never a
// failure; all found give one OK line; no requirements, or a workflow that cannot be used, give none.
func TestDecideRequirements(t *testing.T) {
	i := healthy(t)
	if got := requirementLines(Decide(i, false)); got != "" {
		t.Fatalf("no requirements, no finding:\n%s", got)
	}
	i.Workflow = workflow.Workflow{Requirements: []string{"zz", "go", "aa", "zz"}}
	i.Requirements = map[string]error{"zz": errors.New("not found"), "go": nil, "aa": errors.New("not found")}
	out := Decide(i, false)
	if got := requirementLines(out); got != "warn zz is not on the PATH: polix.yaml requires it\n"+
		"warn aa is not on the PATH: polix.yaml requires it\n" || out.Failed {
		t.Fatalf("missing programs:\n%s", text(out))
	}
	if got := text(out); !strings.Contains(got, "ok workflow polix.yaml is valid\nwarn requirements zz") {
		t.Fatalf("the requirements come right after the workflow:\n%s", got)
	}
	i.Requirements = map[string]error{"zz": nil, "go": nil, "aa": nil, "git": nil}
	if got := requirementLines(Decide(i, false)); got != "ok 4 programs of requirements on the PATH\n" {
		t.Fatalf("all found:\n%s", got)
	}
	i.WorkflowErr, i.Requirements = workflow.InvalidError{"polix.yaml:5: bad"}, nil
	if got := requirementLines(Decide(i, false)); got != "" {
		t.Fatalf("an invalid workflow has no requirements:\n%s", got)
	}
}

func TestFound(t *testing.T) {
	if found("info", "x", "y") != nil || found(setup.LevelOK, "", "y") != nil {
		t.Fatal("an unknown level, or no area, is not a finding")
	}
	f := append(found(setup.LevelOK, "git", "repository at %s", "/r"), found(setup.LevelWarn, "hooks", "old")...)
	if len(f) != 2 || f[0].Message != "repository at /r" || f[0].Problem() || !f[1].Problem() {
		t.Fatalf("%+v", f)
	}
}

var problemRules = []rule.Rule{
	{ID: "A-1", Description: "d", Deterministic: true, Check: "checks/a.sh", File: "ai/rules/a.yaml"},
	{ID: "A-1", Description: "d", File: "ai/rules/b.yaml"},
	{ID: "B-1", Description: "d", Deterministic: true, Check: "tests/b_test.go", File: "ai/rules/b.yaml"},
}

// A check that a runner runs is never reported as not executable; the same error is reported once.
func TestRunProblems(t *testing.T) {
	wf := workflow.Workflow{Rules: workflow.Rules{Run: []workflow.Runner{{Glob: "*_test.go", Argv: []string{"go", "test"}}}}}
	runnable := map[string]error{"checks/a.sh": errors.New("check checks/a.sh is not executable (chmod +x)"),
		"tests/b_test.go": errors.New("check tests/b_test.go is not executable (chmod +x)")}
	got := runProblems(setup.Inspection{Rules: problemRules, Workflow: wf, Runnable: runnable})
	if len(got) != 1 || got[0] != "ai/rules/a.yaml A-1: check checks/a.sh is not executable (chmod +x)" {
		t.Fatalf("%v", got)
	}
	if got := runProblems(setup.Inspection{Rules: problemRules, Runnable: runnable}); len(got) != 2 {
		t.Fatalf("without runner: %v", got)
	}
}
