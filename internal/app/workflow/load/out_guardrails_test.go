package load

import (
	"errors"
	"fmt"
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
  run_on_stop:
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
	}
	return fmt.Sprintf("%s line=%d when=%v %s", e.From, e.Line, e.When, action)
}

// AC6, AC7, AC8, AC9: protected_branches, format_on_edit and run_on_stop expand into the entries, events,
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
		workflow.PostEdit: {
			`guardrails.format_on_edit line=13 when=map[path:*.go] run ["gofmt" "-w" "{path}"] ""`,
			`guardrails.format_on_edit line=15 when=map[] run ["prettier" "--write" "{path}"] "Format failed"`,
		},
		workflow.Stop: {
			`guardrails.run_on_stop line=18 when=map[changed:[*.go go.mod]] run ["go" "test" "./..."] ""`,
			`guardrails.run_on_stop line=20 when=map[] run ["just" "lint"] ""`,
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
	if !slices.Equal(w.ForbiddenCommands, forbidden) {
		t.Errorf("forbidden commands: %+v", w.ForbiddenCommands)
	}
}

// Without shortcuts the entries written in on stay as they are, and a file without on nor shortcuts has no events
// nor protections.
func TestShortcutsWrittenOnly(t *testing.T) {
	w, err := parse(t, "version: 1\non:\n  pre_edit:\n    - action: { block: a }\n    - action: { block: b }\n", "dev")
	if err != nil || len(w.On[workflow.PreEdit]) != 2 || w.On[workflow.PreEdit][0].Action.Block.Message != "a" {
		t.Fatalf("%v %+v", err, w.On)
	}
	w, err = parse(t, "version: 1\nguardrails:\n  protected_files:\n", "dev")
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
		"empty run":         "version: 1\nguardrails:\n  run_on_stop:\n    - run: \"\"\n",
		"run a map":         "version: 1\nguardrails:\n  run_on_stop:\n    - run: { command: x }\n",
		"message not text":  "version: 1\nguardrails:\n  format_on_edit:\n    - { run: x, message: [a] }\n",
		"unknown check key": "version: 1\nguardrails:\n  format_on_edit:\n    - { run: x, changed: \"*.go\" }\n",
		"bad scope":         "version: 1\nguardrails:\n  run_on_stop:\n    - { run: x, changed: { a: b } }\n",
		"bad placeholder":   "version: 1\nguardrails:\n  run_on_stop:\n    - { run: x, message: \"{kye}\" }\n",
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
		"empty run":         "polix.yaml:4: guardrails.run_on_stop: an item needs run",
		"run a map":         "polix.yaml:4: guardrails.run_on_stop: run takes a command, as a string or a list of arguments",
		"message not text":  "polix.yaml:4: guardrails.format_on_edit: message must be a text",
		"unknown check key": `polix.yaml:4: unknown key "changed" in guardrails.format_on_edit (run, path, message)`,
		"bad scope":         "polix.yaml:4: changed: expected a string or a list of strings, got map[a:b]",
		"bad placeholder":   "polix.yaml:4: unknown placeholder {kye} (built in:",
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
			if entryProblem := name == "bad scope" || name == "bad placeholder"; expanded != entryProblem {
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
		"version: 1\nguardrails:\n  run_on_stop:\n    - \"go test ./...\"\n":      `polix.yaml: line 4: an item of guardrails.run_on_stop must be a map, as { run: "go test ./...", changed: "*.go" }`,
		"version: 1\nguardrails:\n  protected_files: []\n  protected_files: []\n": `polix.yaml: line 4: mapping key "protected_files" already defined at line 3`,
	}
	for src, want := range cases {
		_, err := parse(t, src, "dev")
		var invalid workflow.InvalidError
		if err == nil || errors.As(err, &invalid) || err.Error() != want {
			t.Errorf("%q:\ngot  %v\nwant %s", src, err, want)
		}
	}
}

// The section guardrails is decoded as written: each item with its line, and the problems of its values with it.
func TestShortcutsDecodeAsWritten(t *testing.T) {
	src := "version: 1\nguardrails:\n  protected_files: [.env, \"\"]\n  forbidden_commands:\n" +
		"    - { pattern: a, why: x }\n  run_on_stop:\n    - run: go test\n      changed: \"*.go\"\n"
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
	if c := g.RunOnStop; len(c) != 1 || strings.Join(c[0].Run, " ") != "go test" || c[0].Scope != "*.go" ||
		c[0].Line != 7 || c[0].Problems != nil {
		t.Fatalf("run on stop: %+v", c)
	}
}
