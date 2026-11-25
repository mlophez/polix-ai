package adapters

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// shortcutsFile uses every shortcut, with regular entries written in on, on the events they share.
const shortcutsFile = `version: 1
guardrails:
  protected_files:
    - .env
    - "ai/rules/**"
  forbidden_commands:
    - pattern: "rm -rf /*"
    - pattern: "git push --force*"
      reason: "Force pushes rewrite the history of others."
    - pattern: "terraform apply*"
  protected_branches: [main, "release/*"]
  format_on_edit:
    - run: "gofmt -w {path}"
      path: "*.go"
    - run: [prettier, --write, "{path}"]
      message: "Format failed"
  check_on_stop:
    - run: "go test ./..."
      changed: ["*.go", go.mod]
    - run: just lint
on:
  pre_command:
    - action: { block: "regular" }
  pre_commit:
    - action: { block: "regular commit" }
`

// shortcutsSummary describes an entry for the tests: its origin, line, conditions and action.
func shortcutsSummary(e workflow.Entry) string {
	action := ""
	switch {
	case e.Action.Block != nil:
		action = "block " + fmt.Sprintf("%q", e.Action.Block.Message)
	case e.Action.Run != nil:
		action = fmt.Sprintf("run %q %q", e.Action.Run.Command, e.Action.Run.Message)
	case e.Action.Notify != nil:
		action = fmt.Sprintf("notify %q always=%v", e.Action.Notify.Message, e.Action.Notify.Always)
	}
	return fmt.Sprintf("%s line=%d when=%v %s", e.From, e.Line, e.When, action)
}

// waitNotice is the notify of check_on_stop while subagents run, as shortcutsSummary describes it.
const waitNotice = `notify "check_on_stop waits for the subagents still running: its checks run on the next stop after they end." always=true`

// The entries guardrails.builtin expands into when polix.yaml leaves it out, as shortcutsSummary describes them: the
// two of format_polix_files on post_edit and the one of check_polix_files on githooks.pre_commit.
const (
	formatPolixFiles = `guardrails.builtin.format_polix_files line=0 when=map[path:/polix.yaml] run ["polix" "config" "fmt"] ""`
	formatRules      = `guardrails.builtin.format_polix_files line=0 when=map[path:ai/rules/*.yaml] run ["polix" "rules" "fmt"] ""`
	checkPolixFiles  = `guardrails.builtin.check_polix_files line=0 when=map[changed:[/polix.yaml ai/rules/**]] run ["polix" "config" "check"] ""`
)

// writtenForbidden returns the items of forbidden_commands written in the file, without those the presets of
// guardrails.builtin add first.
func writtenForbidden(list []workflow.ForbiddenCommand) []workflow.ForbiddenCommand {
	var out []workflow.ForbiddenCommand
	for _, f := range list {
		if f.From == "" {
			out = append(out, f)
		}
	}
	return out
}

// installAt is the entry of start that installs the hooks of git at line, as shortcutsSummary describes it.
func installAt(line int) string {
	return fmt.Sprintf(`githooks line=%d when=map[] run ["polix" "install" "--githooks"] "The hooks of git of githooks `+
		`are not installed, so the gates of githooks do not apply to the commits:"`, line)
}

// AC12: wait_agents of an item of check_on_stop is true or false, true when left out; an item with wait_agents: false
// expands as before, and the others wait and add the notify.
func TestShortcutsWaitAgents(t *testing.T) {
	src := "version: 1\nguardrails:\n  check_on_stop:\n    - { run: go test, changed: \"*.go\", wait_agents: false }\n" +
		"    - { run: go vet, changed: \"*.go\", wait_agents: true }\n    - { run: just lint }\n"
	var y workflowYAML
	if err := yaml.Unmarshal([]byte(src), &y); err != nil {
		t.Fatal(err)
	}
	g, err := y.Guardrails.guardrails()
	if err != nil || len(g.CheckOnStop) != 3 || g.CheckOnStop[0].Waits || !g.CheckOnStop[1].Waits || !g.CheckOnStop[2].Waits {
		t.Fatalf("check on stop: %+v %v", g.CheckOnStop, err)
	}
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`guardrails.check_on_stop line=4 when=map[changed:*.go] run ["go" "test"] ""`,
		`guardrails.check_on_stop line=5 when=map[agents_are_running:false changed:*.go] run ["go" "vet"] ""`,
		`guardrails.check_on_stop line=6 when=map[agents_are_running:false] run ["just" "lint"] ""`,
		`guardrails.check_on_stop line=5 when=map[agents_are_running:true] ` + waitNotice,
	}
	var got []string
	for _, e := range w.On[workflow.Stop] {
		got = append(got, shortcutsSummary(e))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("stop:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !w.CountsAgents() {
		t.Fatal("the items that wait count the subagents")
	}
	src = "version: 1\nguardrails:\n  format_on_edit:\n    - { run: x, wait_agents: false }\n"
	if _, err := parse(t, src, "dev"); err == nil || !strings.Contains(err.Error(),
		`polix.yaml:4: unknown key "wait_agents" in guardrails.format_on_edit (run, path, message)`) {
		t.Fatalf("format_on_edit: %v", err)
	}
}

// AC6, AC7, AC8, AC9: protected_branches, format_on_edit and check_on_stop expand into the entries, events,
// conditions, messages, lines and origin of the decisions, before the entries written in on; AC16:
// protected_files and forbidden_commands expand into no entry, and the workflow holds them as its protections, in
// file order and with the line of each forbidden command.
func TestShortcutsExpand(t *testing.T) {
	w, err := parse(t, shortcutsFile, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		workflow.PreCommand: {
			` line=23 when=map[] block "regular"`,
		},
		workflow.PreCommit: {
			`guardrails.protected_branches line=11 when=map[branch:^(?:main|release/.*)$] block "{branch} is protected in polix.yaml: create a branch for this change and commit there."`,
			` line=25 when=map[] block "regular commit"`,
		},
		workflow.Start: {installAt(0)},
		workflow.PostEdit: {
			formatPolixFiles,
			formatRules,
			`guardrails.format_on_edit line=13 when=map[path:*.go] run ["gofmt" "-w" "{path}"] ""`,
			`guardrails.format_on_edit line=15 when=map[] run ["prettier" "--write" "{path}"] "Format failed"`,
		},
		workflow.Stop: {
			`guardrails.check_on_stop line=18 when=map[agents_are_running:false changed:[*.go go.mod]] run ["go" "test" "./..."] ""`,
			`guardrails.check_on_stop line=20 when=map[agents_are_running:false] run ["just" "lint"] ""`,
			`guardrails.check_on_stop line=18 when=map[agents_are_running:true] ` + waitNotice,
		},
	}
	if len(w.On) != len(want) {
		t.Fatalf("events: %v", w.On)
	}
	for ev, entries := range want {
		got := make([]string, len(w.On[ev]))
		for i, e := range w.On[ev] {
			got[i] = shortcutsSummary(e)
		}
		if strings.Join(got, "\n") != strings.Join(entries, "\n") {
			t.Errorf("%s:\n%s\nwant:\n%s", ev, strings.Join(got, "\n"), strings.Join(entries, "\n"))
		}
	}
	if !slices.Equal(w.ProtectedFiles, []string{".env", "ai/rules/**"}) {
		t.Errorf("protected files: %q", w.ProtectedFiles)
	}
	forbidden := []workflow.ForbiddenCommand{{Pattern: "rm -rf /*", Line: 7},
		{Pattern: "git push --force*", Reason: "Force pushes rewrite the history of others.", Line: 8},
		{Pattern: "terraform apply*", Line: 10}}
	if !slices.Equal(writtenForbidden(w.ForbiddenCommands), forbidden) {
		t.Errorf("forbidden commands: %+v", w.ForbiddenCommands)
	}
}

// CUJ-06, AC-05: an item of exec_on_start expands into an entry of start without when that runs its command with its
// message and always, after the entry that installs the hooks of git and before the entries written in on.start.
func TestShortcutsExecOnStart(t *testing.T) {
	src := `version: 1
guardrails:
  exec_on_start:
    - run: uv sync --locked --quiet
      message: "uv could not install the test dependencies:"

    - run: [just, setup]
on:
  start:
    - action: { inject: written }
githooks:
  pre_commit:
    - action: { block: x }
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`githooks line=12 when=map[] run ["polix" "install" "--githooks"] "The hooks of git of githooks are not installed, so the gates of githooks do not apply to the commits:"`,
		`guardrails.exec_on_start line=4 when=map[] run ["uv" "sync" "--locked" "--quiet"] "uv could not install the test dependencies:"`,
		`guardrails.exec_on_start line=7 when=map[] run ["just" "setup"] ""`,
		` line=10 when=map[] `,
	}
	var got []string
	for i, e := range w.On[workflow.Start] {
		got = append(got, shortcutsSummary(e))
		if e.Action.Run != nil && !e.Action.Run.Always {
			t.Errorf("entry %d runs without always: %+v", i, e.Action.Run)
		}
		if e.From == "guardrails.exec_on_start" && e.When != nil {
			t.Errorf("entry %d has a when: %v", i, e.When)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("start:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(w.On) != 2 || len(w.On[workflow.PostEdit]) != 2 {
		t.Fatalf("events: start, and post_edit of guardrails.builtin: %v", w.On)
	}
}

// CUJ-05, AC-04: an item of format_on_save expands into an entry of pre_command that runs its command with its
// message on the command lines that stage files (git add or git stage), with its changed when it has one, before the
// entries written in on.pre_command.
func TestShortcutsFormatOnSave(t *testing.T) {
	src := `version: 1
guardrails:
  format_on_save:
    - run: npx eslint --fix
      changed: "*.ts"
      message: "ESLint left problems:"

    - run: [gofmt, -l, .]
on:
  pre_command:
    - action: { block: written }
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`guardrails.format_on_save line=4 when=map[changed:*.ts command:[git add* git stage*]] run ["npx" "eslint" "--fix"] "ESLint left problems:"`,
		`guardrails.format_on_save line=8 when=map[command:[git add* git stage*]] run ["gofmt" "-l" "."] ""`,
		` line=11 when=map[] block "written"`,
	}
	var got []string
	for i, e := range w.On[workflow.PreCommand] {
		got = append(got, shortcutsSummary(e))
		if e.Action.Run != nil && e.Action.Run.Always {
			t.Errorf("entry %d runs with always", i)
		}
	}
	// Besides pre_command, only the entries of guardrails.builtin: the format of the polix files on post_edit and the
	// install of the hooks of git its check needs on start.
	if strings.Join(got, "\n") != strings.Join(want, "\n") || len(w.On) != 3 || len(w.On[workflow.PostEdit]) != 2 ||
		len(w.On[workflow.Start]) != 1 {
		t.Fatalf("pre_command:\n%s\nwant:\n%s\nevents: %v", strings.Join(got, "\n"), strings.Join(want, "\n"), w.On)
	}
	_, err = parse(t, "version: 1\nguardrails:\n  format_on_save:\n    - { run: x, path: \"*.ts\" }\n", "dev")
	if err == nil || !strings.Contains(err.Error(),
		`polix.yaml:4: unknown key "path" in guardrails.format_on_save (run, changed, message)`) {
		t.Fatalf("the keys of an item: %v", err)
	}
}

// CUJ-03, AC-02, AC-01: an item of check_on_save expands into an entry of githooks.pre_commit that runs its command
// with its message, with its changed when it has one, before the entries written there; a file with only
// check_on_save installs the hooks of git first in start at the line of its first item, an entry polix config fmt
// never writes into the file.
func TestShortcutsCheckOnSave(t *testing.T) {
	src := `version: 1
guardrails:
  builtin: { format_polix_files: false, check_polix_files: false }
  check_on_save:
    - run: golangci-lint run ./...
      changed: ["*.go", go.mod]
      message: "Fix the lint before committing:"

    - run: polix config check
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if start := w.On[workflow.Start]; len(start) != 1 || shortcutsSummary(start[0]) != installAt(5) ||
		!start[0].Action.Run.Always {
		t.Fatalf("start: %+v", start)
	}
	want := []string{
		`guardrails.check_on_save line=5 when=map[changed:[*.go go.mod]] run ["golangci-lint" "run" "./..."] "Fix the lint before committing:"`,
		`guardrails.check_on_save line=9 when=map[] run ["polix" "config" "check"] ""`,
	}
	var got []string
	for _, e := range w.Githooks[workflow.PreCommit] {
		got = append(got, shortcutsSummary(e))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || len(w.Githooks) != 1 {
		t.Fatalf("githooks:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(w.On) != 1 {
		t.Fatalf("check_on_save adds no entry of on but the install: %v", w.On)
	}
	// The entries of check_on_save come before those written in githooks.pre_commit, and the install takes the line
	// of the first event of githooks.
	w, err = parse(t, src+"githooks:\n  pre_commit:\n    - action: { block: written }\n", "dev")
	if err != nil {
		t.Fatal(err)
	}
	got = nil
	for _, e := range w.Githooks[workflow.PreCommit] {
		got = append(got, shortcutsSummary(e))
	}
	if want := append(want, ` line=12 when=map[] block "written"`); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("githooks with written entries:\n%s", strings.Join(got, "\n"))
	}
	if start := w.On[workflow.Start]; len(start) != 1 || start[0].Line != 11 {
		t.Fatalf("the install at the line of githooks.pre_commit: %+v", start)
	}
}

// An item of a shortcut reads a value written as an alias as the value of its anchor, as an entry of on does: a
// check of check_on_save reuses the command and the globs of check_on_stop, which comes before it.
func TestShortcutsAliases(t *testing.T) {
	src := `version: 1
guardrails:
  protected_files: &protected [.env]
  forbidden_commands:
    - &push { pattern: "git push --force*", reason: &why "No force pushes" }
  check_on_stop:
    - run: &lint golangci-lint run ./...
      changed: &go ["*.go", go.mod]
      message: *why
  check_on_save:
    - run: *lint
      changed: *go
on:
  pre_edit:
    - when: { path: *protected }
      action: { block: x }
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := `guardrails.check_on_save line=11 when=map[changed:[*.go go.mod]] run ["golangci-lint" "run" "./..."] ""`
	if got := w.Githooks[workflow.PreCommit]; len(got) != 2 || shortcutsSummary(got[0]) != checkPolixFiles ||
		shortcutsSummary(got[1]) != want {
		t.Fatalf("check_on_save: %+v", got)
	}
	if stop := w.On[workflow.Stop]; len(stop) == 0 || stop[0].Action.Run.Message != "No force pushes" {
		t.Fatalf("check_on_stop: %+v", stop)
	}
	src = strings.Replace(src, "  check_on_save:", "  protected_branches: *protected\n  check_on_save:", 1)
	src = strings.Replace(src, "  check_on_stop:", "  exec_on_start:\n    - *push\n  check_on_stop:", 1)
	if _, err := parse(t, src, "dev"); err == nil || !strings.Contains(err.Error(),
		`unknown key "pattern" in guardrails.exec_on_start (run, message)`) {
		t.Fatalf("an alias to a map is read as that map: %v", err)
	}
	// An alias among the arguments of a run reads as its anchor in every lifecycle shortcut, as in an entry of on.
	for _, shortcut := range []string{workflow.ShortcutExecOnStart, workflow.ShortcutFormatOnEdit,
		workflow.ShortcutFormatOnSave, workflow.ShortcutCheckOnStop, workflow.ShortcutCheckOnSave} {
		src := "version: 1\nguardrails:\n  " + shortcut + ":\n    - run: [&bin golangci-lint, version]\n" +
			"    - run: [*bin, run, ./...]\n"
		w, err := parse(t, src, "dev")
		if err != nil {
			t.Fatalf("%s: an alias in a run list: %v", shortcut, err)
		}
		found := false
		for _, entries := range append(slices.Collect(maps.Values(w.On)), w.Githooks[workflow.PreCommit]) {
			for _, e := range entries {
				found = found || e.Line == 5 && e.Action.Run != nil &&
					slices.Equal(e.Action.Run.Command, []string{"golangci-lint", "run", "./..."})
			}
		}
		if !found {
			t.Fatalf("%s: no entry runs the aliased command: %+v %+v", shortcut, w.On, w.Githooks)
		}
	}
}

// An aliased item, or an item of an aliased list, takes the line where its shortcut writes it, not the line of its
// anchor in another shortcut: the alias of the list, even when its items are aliases too, else the alias of the item,
// else the item. An alias among the strings of protected_files or protected_branches reads as the string of its
// anchor.
func TestShortcutsAliasLines(t *testing.T) {
	src := `version: 1
guardrails:
  protected_files: &files [&main main, &rel "release/*"]
  check_on_stop: &checks
    - &lint
      run: golangci-lint run ./...
  protected_branches:
    - *main
    - *rel
  check_on_save:
    - *lint
  format_on_save: *checks
  exec_on_start: *checks
  forbidden_commands:
    - &push { pattern: "git push --force*" }
    - *push
`
	w, err := parse(t, src, "dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		checkPolixFiles,
		`guardrails.check_on_save line=11 when=map[] run ["golangci-lint" "run" "./..."] ""`,
		`guardrails.protected_branches line=8 when=map[branch:^(?:main|release/.*)$] block "{branch} is ` +
			`protected in polix.yaml: create a branch for this change and commit there."`,
	}
	got := []string{}
	for _, e := range w.Githooks[workflow.PreCommit] {
		got = append(got, shortcutsSummary(e))
	}
	for _, e := range w.On[workflow.PreCommit] {
		got = append(got, shortcutsSummary(e))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("pre_commit:\n%s", strings.Join(got, "\n"))
	}
	lines := map[string]int{}
	for _, entries := range w.On {
		for _, e := range entries {
			lines[e.From] = e.Line
		}
	}
	if lines["guardrails.format_on_save"] != 12 || lines["guardrails.exec_on_start"] != 13 {
		t.Fatalf("an item of an aliased list takes the line of the alias: %v", lines)
	}
	pushes := []int{}
	for _, f := range writtenForbidden(w.ForbiddenCommands) {
		pushes = append(pushes, f.Line)
	}
	if !slices.Equal(pushes, []int{15, 16}) {
		t.Fatalf("an aliased item of forbidden_commands takes the line of its alias: %v", pushes)
	}

	problems := map[string]string{
		"aliased items": `version: 1
guardrails:
  protected_files: [&e ""]
  protected_branches:
    - *e
  check_on_stop:
    - &lint
      run: golangci-lint run ./...
      wait_agents: false
  check_on_save:
    - *lint
  forbidden_commands:
    - &bad { reason: x }
    - *bad
`,
		"aliased lists": `version: 1
guardrails:
  protected_files: &p [""]
  check_on_stop: &checks
    - run: golangci-lint run ./...
      wait_agents: false
  protected_branches: *p
  check_on_save: *checks
`,
		"aliased lists of aliases": `version: 1
guardrails:
  protected_branches: &pb [&e "", *e]
  protected_files: *pb
  check_on_stop:
    - &lint
      run: golangci-lint run ./...
      wait_agents: false
  format_on_save: &saved
    - *lint
  check_on_save: *saved
`,
	}
	wants := map[string][]string{
		"aliased items": {
			"polix.yaml:3: guardrails.protected_files: a path glob cannot be empty",
			"polix.yaml:5: guardrails.protected_branches: a branch name cannot be empty",
			`polix.yaml:11: unknown key "wait_agents" in guardrails.check_on_save`,
			"polix.yaml:13: guardrails.forbidden_commands: an item needs a pattern",
			"polix.yaml:14: guardrails.forbidden_commands: an item needs a pattern",
		},
		"aliased lists": {
			"polix.yaml:3: guardrails.protected_files: a path glob cannot be empty",
			"polix.yaml:7: guardrails.protected_branches: a branch name cannot be empty",
			`polix.yaml:8: unknown key "wait_agents" in guardrails.check_on_save`,
		},
		"aliased lists of aliases": {
			"polix.yaml:3: guardrails.protected_branches: a branch name cannot be empty",
			"polix.yaml:4: guardrails.protected_files: a path glob cannot be empty",
			`polix.yaml:10: unknown key "wait_agents" in guardrails.format_on_save`,
			`polix.yaml:11: unknown key "wait_agents" in guardrails.check_on_save`,
		},
	}
	for name, src := range problems {
		_, err := parse(t, src, "dev")
		if err == nil {
			t.Fatalf("%s: no problems", name)
		}
		for _, w := range wants[name] {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: want %q in:\n%v", name, w, err)
			}
		}
		if name == "aliased lists of aliases" && strings.Contains(err.Error(), "polix.yaml:3: guardrails.protected_files") {
			t.Errorf("%s: an item of protected_files reported at its anchor in protected_branches:\n%v", name, err)
		}
		reported := 0
		for _, w := range wants[name] {
			reported += strings.Count(w, "wait_agents")
		}
		if strings.Count(err.Error(), "wait_agents") != reported {
			t.Errorf("%s: wait_agents reported %d times, want %d:\n%v", name,
				strings.Count(err.Error(), "wait_agents"), reported, err)
		}
	}
}

// Without shortcuts the entries written in on stay as they are, and a file without on nor shortcuts has no events
// nor protections.
func TestShortcutsWrittenOnly(t *testing.T) {
	w, err := parse(t, "version: 1\non:\n  pre_edit:\n    - action: { block: { message: a, once: true } }\n"+
		"    - action: { block: b }\n", "dev")
	if err != nil || len(w.On[workflow.PreEdit]) != 2 || w.On[workflow.PreEdit][0].Action.Block.Message != "a" {
		t.Fatalf("%v %+v", err, w.On)
	}
	off := "  builtin: { forbid_commit_no_verify: false, forbid_git_destructive_commands: false, " +
		"format_polix_files: false, check_polix_files: false }\n"
	w, err = parse(t, "version: 1\nguardrails:\n"+off+"  protected_files:\n", "dev")
	if err != nil || w.On != nil || w.ProtectedFiles != nil || w.ForbiddenCommands != nil {
		t.Fatalf("an empty shortcut: %v %+v", err, w)
	}
}

// AC15: each problem of a value is reported at the line of its item, with the rest of the problems of the file, and
// an item with problems expands into nothing.
func TestShortcutsProblems(t *testing.T) {
	cases := map[string]string{
		"empty glob": "version: 1\nguardrails:\n  protected_files:\n    - \"\"\n    - .env\n" +
			"  forbidden_commands:\n    - { pattern: \"\" }\n",
		"empty branch":      "version: 1\nguardrails:\n  protected_branches:\n    - main\n    - \"\"\n",
		"no pattern":        "version: 1\nguardrails:\n  forbidden_commands:\n    - reason: x\n",
		"pattern not text":  "version: 1\nguardrails:\n  forbidden_commands:\n    - pattern: [a]\n",
		"reason not text":   "version: 1\nguardrails:\n  forbidden_commands:\n    - { pattern: a, reason: { x: y } }\n",
		"unknown item key":  "version: 1\nguardrails:\n  forbidden_commands:\n    - { pattern: a, why: x }\n",
		"repeated item key": "version: 1\nguardrails:\n  forbidden_commands:\n    - pattern: a\n      pattern: b\n",
		"no run":            "version: 1\nguardrails:\n  format_on_edit:\n    - path: \"*.go\"\n",
		"empty run":         "version: 1\nguardrails:\n  check_on_stop:\n    - run: \"\"\n",
		"run a map":         "version: 1\nguardrails:\n  check_on_stop:\n    - run: { command: x }\n",
		"message not text":  "version: 1\nguardrails:\n  format_on_edit:\n    - { run: x, message: [a] }\n",
		"unknown check key": "version: 1\nguardrails:\n  format_on_edit:\n    - { run: x, changed: \"*.go\" }\n",
		"bad scope":         "version: 1\nguardrails:\n  check_on_stop:\n    - { run: x, changed: { a: b } }\n",
		"bad placeholder":   "version: 1\nguardrails:\n  check_on_stop:\n    - { run: x, message: \"{kye}\" }\n",
		"wait_agents text":  "version: 1\nguardrails:\n  check_on_stop:\n    - { run: x, wait_agents: \"no\" }\n",
		"wait_agents list":  "version: 1\nguardrails:\n  check_on_stop:\n    - run: x\n      wait_agents: [false]\n",
		"exec always":       "version: 1\nguardrails:\n  exec_on_start:\n    - { run: uv sync, always: true }\n",
		"exec changed":      "version: 1\nguardrails:\n  exec_on_start:\n    - { run: uv sync, changed: uv.lock }\n",
		"exec no run":       "version: 1\nguardrails:\n  exec_on_start:\n    - message: x\n",
		"save placeholder":  "version: 1\nguardrails:\n  check_on_save:\n    - run: gofmt -l {path}\n",
		"save wait_agents":  "version: 1\nguardrails:\n  check_on_save:\n    - { run: go test, wait_agents: false }\n",
		"save no run":       "version: 1\nguardrails:\n  check_on_save:\n    - changed: \"*.go\"\n",
	}
	want := map[string]string{
		"empty glob": "polix.yaml:4: guardrails.protected_files: a path glob cannot be empty\n" +
			"polix.yaml:7: guardrails.forbidden_commands: an item needs a pattern",
		"empty branch":      "polix.yaml:5: guardrails.protected_branches: a branch name cannot be empty",
		"no pattern":        "polix.yaml:4: guardrails.forbidden_commands: an item needs a pattern",
		"pattern not text":  "polix.yaml:4: guardrails.forbidden_commands: pattern must be a text",
		"reason not text":   "polix.yaml:4: guardrails.forbidden_commands: reason must be a text",
		"unknown item key":  `polix.yaml:4: unknown key "why" in guardrails.forbidden_commands (pattern, reason)`,
		"repeated item key": `polix.yaml:4: guardrails.forbidden_commands: key "pattern" already defined at line 4`,
		"no run":            "polix.yaml:4: guardrails.format_on_edit: an item needs run",
		"empty run":         "polix.yaml:4: guardrails.check_on_stop: an item needs run",
		"run a map":         "polix.yaml:4: guardrails.check_on_stop: run takes a command, as a string or a list of arguments",
		"message not text":  "polix.yaml:4: guardrails.format_on_edit: message must be a text",
		"unknown check key": `polix.yaml:4: unknown key "changed" in guardrails.format_on_edit (run, path, message)`,
		"bad scope":         "polix.yaml:4: changed: expected a string or a list of strings, got map[a:b]",
		"bad placeholder":   "polix.yaml:4: unknown placeholder {kye} (built in:",
		"wait_agents text":  "polix.yaml:4: guardrails.check_on_stop: wait_agents must be true or false",
		"wait_agents list":  "polix.yaml:4: guardrails.check_on_stop: wait_agents must be true or false",
		"exec always":       `polix.yaml:4: unknown key "always" in guardrails.exec_on_start (run, message)`,
		"exec changed":      `polix.yaml:4: unknown key "changed" in guardrails.exec_on_start (run, message)`,
		"exec no run":       "polix.yaml:4: guardrails.exec_on_start: an item needs run",
		"save placeholder":  "polix.yaml:4: guardrails.check_on_save: unknown placeholder {path} (githooks allow {branch}, or a variable of vars)",
		"save wait_agents":  `polix.yaml:4: unknown key "wait_agents" in guardrails.check_on_save (run, changed, message)`,
		"save no run":       "polix.yaml:4: guardrails.check_on_save: an item needs run",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			w, err := parse(t, src, "dev")
			var invalid workflow.InvalidError
			if !errors.As(err, &invalid) || !strings.HasPrefix(err.Error(), want[name]) {
				t.Fatalf("got %v, want %s", err, want[name])
			}
			if strings.Contains(err.Error(), "run needs exactly one") {
				t.Fatalf("a problem of an item is reported twice: %v", err)
			}
			// CUJ-04: a problem of check_on_save names what git can never give its hooks, never an option of a newer
			// polix.
			for line := range strings.SplitSeq(err.Error(), "\n") {
				if strings.HasPrefix(name, "save ") && strings.Contains(line, "unknown") &&
					!workflow.Problem(line).Known() {
					t.Fatalf("a problem of check_on_save may be an option of a newer polix: %s", line)
				}
			}
			// The problem of a value of the item keeps it out of the entries and the protections; the problem of a
			// condition or a message is one of the entry it expands into.
			var line int
			if _, err := fmt.Sscanf(want[name], "polix.yaml:%d:", &line); err != nil {
				t.Fatal(err)
			}
			expanded := false
			for _, entries := range w.On {
				for _, e := range entries {
					expanded = expanded || e.Line == line
				}
			}
			for _, f := range w.ForbiddenCommands {
				expanded = expanded || f.Line == line
			}
			for _, e := range w.Githooks[workflow.PreCommit] {
				expanded = expanded || e.Line == line
			}
			entryProblem := name == "bad scope" || name == "bad placeholder" || name == "save placeholder"
			if expanded != entryProblem {
				t.Fatalf("the item at line %d expanded: %v, %+v", line, expanded, w.On)
			}
		})
	}
	// The problems of the shortcuts come in the order of the file with those of the workflow.
	_, err := parse(t, "version: 2\nguardrails:\n  protected_files: [\"\"]\non:\n  pre_edit:\n"+
		"    - action: { blok: x }\n", "dev")
	if err == nil || !strings.HasPrefix(err.Error(), "polix.yaml: version must be 1, got 2\n"+
		"polix.yaml:3: guardrails.protected_files: a path glob cannot be empty\npolix.yaml:6: ") {
		t.Fatalf("order: %v", err)
	}
}

// AC15: a shortcut in another form than its own fails the parse at its line with the form it takes, never with an
// error of yaml.
func TestShortcutsForms(t *testing.T) {
	cases := map[string]string{
		"version: 1\nguardrails:\n  protected_files: .env\n":                      `polix.yaml: line 3: guardrails.protected_files takes a list of path globs, as [".env", "secrets/**"]`,
		"version: 1\nguardrails:\n  protected_files: [[a]]\n":                     `polix.yaml: line 3: guardrails.protected_files takes a list of path globs, as [".env", "secrets/**"]`,
		"version: 1\nguardrails:\n  protected_branches: main\n":                   `polix.yaml: line 3: guardrails.protected_branches takes a list of branch names, as [main, "release/*"]`,
		"version: 1\nguardrails:\n  forbidden_commands:\n    - \"rm -rf /*\"\n":   `polix.yaml: line 4: an item of guardrails.forbidden_commands must be a map, as { pattern: "git push --force*", reason: "..." }`,
		"version: 1\nguardrails:\n  forbidden_commands: { pattern: x }\n":         `polix.yaml: line 3: guardrails.forbidden_commands takes a list of maps, each as { pattern: "git push --force*", reason: "..." }`,
		"version: 1\nguardrails:\n  format_on_edit:\n    - \"gofmt -w {path}\"\n": `polix.yaml: line 4: an item of guardrails.format_on_edit must be a map, as { run: "gofmt -w {path}", path: "*.go" }`,
		"version: 1\nguardrails:\n  check_on_stop:\n    - \"go test ./...\"\n":    `polix.yaml: line 4: an item of guardrails.check_on_stop must be a map, as { run: "go test ./...", changed: "*.go" }`,
		"version: 1\nguardrails:\n  protected_files: []\n  protected_files: []\n": `polix.yaml: line 4: mapping key "protected_files" already defined at line 3`,
		"version: 1\nguardrails:\n  exec_on_start: uv sync --locked\n":            `polix.yaml: line 3: guardrails.exec_on_start takes a list of maps, each as { run: "uv sync --locked" }`,
		"version: 1\nguardrails:\n  exec_on_start:\n    - uv sync --locked\n":     `polix.yaml: line 4: an item of guardrails.exec_on_start must be a map, as { run: "uv sync --locked" }`,
		"version: 1\nguardrails:\n  format_on_save: npx eslint --fix\n":           `polix.yaml: line 3: guardrails.format_on_save takes a list of maps, each as { run: "npx eslint --fix", changed: "*.ts" }`,
		"version: 1\nguardrails:\n  check_on_save: go test ./...\n":               `polix.yaml: line 3: guardrails.check_on_save takes a list of maps, each as { run: "go test ./...", changed: "*.go" }`,
	}
	for src, want := range cases {
		_, err := parse(t, src, "dev")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q:\ngot  %v\nwant %s", src, err, want)
		}
	}
}

// CUJ-07, AC-06: run_on_stop, the old name of check_on_stop, fails the parse at its line as an unknown key of
// guardrails, with the shortcuts in their order and the hint of its new name, so it is a name this polix renamed and
// never an option of a newer polix.
func TestShortcutsRenamed(t *testing.T) {
	src := "version: 1\nguardrails:\n  run_on_stop:\n    - run: go test ./...\n      changed: \"*.go\"\n"
	_, err := parse(t, src, "dev")
	want := `polix.yaml: line 3: unknown key "run_on_stop" in guardrails (builtin, protected_files, protected_branches, ` +
		`forbidden_commands, exec_on_start, format_on_edit, format_on_save, check_on_stop, check_on_save): use check_on_stop`
	var invalid workflow.InvalidError
	if err == nil || errors.As(err, &invalid) || err.Error() != want {
		t.Fatalf("got  %v\nwant %s", err, want)
	}
	if !workflow.Problem(err.Error()).Renamed() {
		t.Fatalf("not renamed: %v", err)
	}
	_, err = parse(t, "version: 1\nguardrails:\n  run_on_save: []\n", "dev")
	if err == nil || strings.Contains(err.Error(), ": use ") || workflow.Problem(err.Error()).Renamed() {
		t.Fatalf("a key this polix never had is not renamed: %v", err)
	}
}

// The section guardrails is decoded as written: each item with its line, and the problems of its values with it.
func TestShortcutsDecodeAsWritten(t *testing.T) {
	src := "version: 1\nguardrails:\n  protected_files: [.env, \"\"]\n  forbidden_commands:\n" +
		"    - { pattern: a, why: x }\n  check_on_stop:\n    - run: go test\n      changed: \"*.go\"\n"
	var y workflowYAML
	if err := yaml.Unmarshal([]byte(src), &y); err != nil {
		t.Fatal(err)
	}
	g, err := y.Guardrails.guardrails()
	if err == nil || len(g.ProtectedFiles) != 2 || g.ProtectedFiles[1] != (workflow.GuardrailItem{Line: 3}) {
		t.Fatalf("protected files: %+v %v", g.ProtectedFiles, err)
	}
	if f := g.ForbiddenCommands; len(f) != 1 || f[0].Pattern != "a" || f[0].Line != 5 ||
		strings.Join(f[0].Problems, "|") != `unknown key "why" in guardrails.forbidden_commands (pattern, reason)` {
		t.Fatalf("forbidden commands: %+v", f)
	}
	if c := g.CheckOnStop; len(c) != 1 || strings.Join(c[0].Run, " ") != "go test" || c[0].Scope != "*.go" ||
		c[0].Line != 7 || c[0].Problems != nil {
		t.Fatalf("check on stop: %+v", c)
	}
}

// CUJ-09, AC-06: a value of guardrails.builtin other than true or false (LIT-05) and an unknown key of it (LIT-06)
// are problems at the line of their key, the practice of a wrong value staying on, and only the unknown key may be
// an option of a newer polix; a builtin that is not a map is the error of LIT-07. A valid map loads its switches,
// each one it leaves out on, as are all four without builtin or with null.
func TestBuiltinProblems(t *testing.T) {
	for src, want := range map[string]string{
		"version: 1\nguardrails:\n  builtin:\n    format_polix_files: \"yes\"\n": "polix.yaml:4: guardrails.builtin: format_polix_files must be true or false",
		"version: 1\nguardrails:\n  builtin:\n    check_polix_files: true\n    bogus: false\n": `polix.yaml:5: unknown key "bogus" in guardrails.builtin ` +
			"(forbid_commit_no_verify, forbid_git_destructive_commands, format_polix_files, check_polix_files)",
	} {
		w, err := parse(t, src, "dev")
		var invalid workflow.InvalidError
		if !errors.As(err, &invalid) || strings.Join(invalid, "\n") != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
			continue
		}
		if !w.Builtin.FormatPolixFiles || !w.Builtin.CheckPolixFiles {
			t.Errorf("%q: the practice stays on: %+v", src, w.Builtin)
		}
	}
	if workflow.Problem(`polix.yaml:5: unknown key "bogus" in guardrails.builtin (x)`).Known() {
		t.Error("an unknown key of builtin may be an option of a newer polix")
	}
	for _, src := range []string{"version: 1\nguardrails:\n  builtin: true\n", "version: 1\nguardrails:\n  builtin: [x]\n"} {
		_, err := parse(t, src, "dev")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || err.Error() != "polix.yaml: line 3: guardrails.builtin takes a map "+
			"of true or false, as { forbid_git_destructive_commands: false }" {
			t.Errorf("%q: LIT-07: %v", src, err)
		}
	}
	w, err := parse(t, "version: 1\nguardrails:\n  builtin: { forbid_git_destructive_commands: false, "+
		"check_polix_files: false }\n", "dev")
	if b := w.Builtin; err != nil || !b.ForbidCommitNoVerify || b.ForbidGitDestructiveCommands || !b.FormatPolixFiles ||
		b.CheckPolixFiles || b.Line != 3 || len(w.Githooks) != 0 || len(w.On[workflow.Start]) != 0 {
		t.Fatalf("a valid map: %v %+v", err, w)
	}
	for _, src := range []string{"version: 1\n", "version: 1\nguardrails:\n  builtin:\n"} {
		w, err := parse(t, src, "dev")
		if b := w.Builtin; err != nil || !b.ForbidCommitNoVerify || !b.ForbidGitDestructiveCommands ||
			!b.FormatPolixFiles || !b.CheckPolixFiles || len(w.On[workflow.PostEdit]) != 2 ||
			len(w.Githooks[workflow.PreCommit]) != 1 || len(w.On[workflow.Start]) != 1 {
			t.Errorf("%q: all on: %v %+v", src, err, w)
		}
	}
}

// FND-06: a path that cannot be the folder of the AI is its only problem: the built-in format and check of the polix
// files are not built from it, so no placeholder of it is reported and no unknown name is followed by the advice to
// update polix.
func TestBuiltinInvalidPath(t *testing.T) {
	_, err := parse(t, "version: 1\npath: \"a{b}\"\n", "dev")
	var invalid workflow.InvalidError
	if !errors.As(err, &invalid) || len(invalid) != 1 || !strings.Contains(invalid[0], `path "a{b}"`) {
		t.Fatalf("got %v, want only the problem of path", err)
	}
	for _, p := range invalid {
		if strings.Contains(p, "unknown") && !workflow.Problem(p).Known() {
			t.Errorf("the problem would suggest a newer polix: %s", p)
		}
	}
}

// CUJ-11, AC-07: also_on_save of an item of check_on_stop takes true or false (LIT-08), and with true the strings of
// the item are checked with the vocabulary of githooks, so a placeholder git can never give is a problem (LIT-09);
// both come at the line of the item and are names this polix knows, never options of a newer polix. With
// also_on_save false the same item is valid, and with true it loads also on githooks.pre_commit.
func TestAlsoOnSaveProblems(t *testing.T) {
	const item = "version: 1\nguardrails:\n  check_on_stop:\n    - run: %s\n      also_on_save: %s\n"
	for src, want := range map[string]string{
		fmt.Sprintf(item, "go vet ./...", "3"): "polix.yaml:4: guardrails.check_on_stop: also_on_save must be true or false",
		fmt.Sprintf(item, "echo {path}", "true"): "polix.yaml:4: placeholder {path} never expands on stop (only on pre_edit, post_edit)\n" +
			"polix.yaml:4: guardrails.check_on_stop: unknown placeholder {path} (githooks allow {branch}, or a variable of vars)",
	} {
		_, err := parse(t, src, "dev")
		var invalid workflow.InvalidError
		if !errors.As(err, &invalid) || strings.Join(invalid, "\n") != want {
			t.Errorf("%q: got %v, want %s", src, err, want)
			continue
		}
		for p := range strings.SplitSeq(want, "\n") {
			if strings.Contains(p, "placeholder") && !workflow.Problem(p).Known() {
				t.Errorf("%q: the problem would suggest a newer polix", src)
			}
		}
	}
	if _, err := parse(t, fmt.Sprintf(item, "echo {risk_score}", "false"), "dev"); err != nil {
		t.Errorf("also_on_save false: %v", err)
	}
	w, err := parse(t, fmt.Sprintf(item, "go vet ./...", "true"), "dev")
	if pre := w.Githooks[workflow.PreCommit]; err != nil || len(pre) != 2 || pre[1].From != "guardrails.check_on_stop" ||
		pre[1].Line != 4 {
		t.Fatalf("also_on_save true: %v %+v", err, pre)
	}
}

// CUJ-15, AC-10: a written format_on_edit item for polix config fmt and a check_on_save item stay, after the entries
// of format_polix_files on post_edit and of check_polix_files on githooks.pre_commit.
func TestBuiltinKeepsWritten(t *testing.T) {
	w, err := parse(t, `version: 1
guardrails:
  forbidden_commands:
    - pattern: "kubectl* apply *"
      reason: Apply through the pipeline.
  format_on_edit:
    - run: polix config fmt
      path: polix.yaml
  check_on_save:
    - run: go test ./...
`, "dev")
	if err != nil {
		t.Fatal(err)
	}
	var edit, save []string
	for _, e := range w.On[workflow.PostEdit] {
		edit = append(edit, e.From+" "+strings.Join(e.Action.Run.Command, " "))
	}
	for _, e := range w.Githooks[workflow.PreCommit] {
		save = append(save, e.From+" "+strings.Join(e.Action.Run.Command, " "))
	}
	if want := []string{"guardrails.builtin.format_polix_files polix config fmt",
		"guardrails.builtin.format_polix_files polix rules fmt", "guardrails.format_on_edit polix config fmt"}; !slices.Equal(edit, want) {
		t.Errorf("post_edit: %q", edit)
	}
	if want := []string{"guardrails.builtin.check_polix_files polix config check",
		"guardrails.check_on_save go test ./..."}; !slices.Equal(save, want) {
		t.Errorf("githooks.pre_commit: %q", save)
	}
	if f := w.ForbiddenCommands; len(f) == 0 || f[len(f)-1].Reason != "Apply through the pipeline." ||
		len(writtenForbidden(f)) != 1 || f[0].From != "guardrails.builtin.forbid_commit_no_verify" {
		t.Errorf("forbidden commands: %+v", f)
	}
}
