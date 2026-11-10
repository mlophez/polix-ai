package workflow

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// The decoding of polix.yaml and its cases, the files that use every shortcut included, are tested in
// internal/app/workflow/load; these tests build the section as written.

// shortcutsSummary describes an entry for the tests: its origin, line, conditions and action.
func shortcutsSummary(e Entry) string {
	action := ""
	switch {
	case e.Action.Block != nil:
		action = "block " + fmt.Sprintf("%q", e.Action.Block.Message)
	case e.Action.Run != nil:
		action = fmt.Sprintf("run %q %q", e.Action.Run.Command, e.Action.Run.Message)
	}
	return fmt.Sprintf("%s line=%d when=%v %s", e.From, e.Line, e.When, action)
}

// protected_branches, format_on_edit and run_on_stop expand into entries before those written in on, in file
// order and with the line of their item; an empty name or an item with problems expands into nothing, and without
// them on stays as it is.
func TestGuardrailsExpand(t *testing.T) {
	written := map[string][]Entry{PreCommit: {{Action: Action{Block: &Block{Message: "regular"}}, Line: 25}}}
	g := Guardrails{
		ProtectedBranches: []GuardrailItem{{Text: "", Line: 10}, {Text: "main", Line: 11}, {Text: "release/*", Line: 12}},
		FormatOnEdit: []GuardrailCheck{{Run: []string{"gofmt", "-w", "{path}"}, Scope: "*.go", Line: 13},
			{Run: []string{"x"}, Line: 14, Problems: []string{"guardrails.format_on_edit: message must be a text"}},
			{Run: []string{"prettier"}, Message: "Format failed", Line: 15}},
		RunOnStop: []GuardrailCheck{{Run: []string{"go", "test", "./..."}, Scope: []any{"*.go"}, Line: 18}},
	}
	want := map[string][]string{
		PreCommit: {
			`guardrails.protected_branches line=11 when=map[branch:^(?:main|release/.*)$] block "{branch} is protected in polix.yaml: create a branch for this change and commit there."`,
			` line=25 when=map[] block "regular"`,
		},
		PostEdit: {
			`guardrails.format_on_edit line=13 when=map[path:*.go] run ["gofmt" "-w" "{path}"] ""`,
			`guardrails.format_on_edit line=15 when=map[] run ["prettier"] "Format failed"`,
		},
		Stop: {`guardrails.run_on_stop line=18 when=map[changed:[*.go]] run ["go" "test" "./..."] ""`},
	}
	on := g.Expand(written)
	if len(on) != len(want) || len(written[PreCommit]) != 1 {
		t.Fatalf("events: %v; written %v", on, written)
	}
	for ev, entries := range want {
		got := make([]string, len(on[ev]))
		for i, e := range on[ev] {
			got[i] = shortcutsSummary(e)
		}
		if strings.Join(got, "\n") != strings.Join(entries, "\n") {
			t.Errorf("%s:\n%s\nwant:\n%s", ev, strings.Join(got, "\n"), strings.Join(entries, "\n"))
		}
	}
	if got := (Guardrails{}).Expand(written); len(got) != 1 || len(got[PreCommit]) != 1 {
		t.Fatalf("no shortcuts: %v", got)
	}
}

// protected_files and forbidden_commands are protections: the globs that are not empty and the items without
// problems, in file order.
func TestGuardrailsProtections(t *testing.T) {
	g := Guardrails{
		ProtectedFiles: []GuardrailItem{{Text: ".env", Line: 3}, {Text: " ", Line: 4}, {Text: "ai/rules/**", Line: 5}},
		ForbiddenCommands: []GuardrailForbidden{{Pattern: "rm -rf /*", Line: 7},
			{Pattern: "a", Line: 8, Problems: []string{"guardrails.forbidden_commands: reason must be a text"}},
			{Pattern: "git push --force*", Reason: "Force pushes rewrite the history of others.", Line: 9}},
	}
	if got := g.Protected(); !slices.Equal(got, []string{".env", "ai/rules/**"}) {
		t.Errorf("protected files: %q", got)
	}
	want := []ForbiddenCommand{{Pattern: "rm -rf /*", Line: 7},
		{Pattern: "git push --force*", Reason: "Force pushes rewrite the history of others.", Line: 9}}
	if got := g.Forbidden(); !slices.Equal(got, want) {
		t.Errorf("forbidden commands: %+v", got)
	}
	if (Guardrails{}).Protected() != nil || (Guardrails{}).Forbidden() != nil {
		t.Fatal("no protections")
	}
}

// The problems of the items come at their lines with those of the workflow, in the order of the file; without them
// there are none, whatever the workflow.
func TestGuardrailsProblems(t *testing.T) {
	g := Guardrails{
		ProtectedFiles:    []GuardrailItem{{Text: "", Line: 3}},
		ForbiddenCommands: []GuardrailForbidden{{Line: 7, Problems: []string{"guardrails.forbidden_commands: an item needs a pattern"}}},
		ProtectedBranches: []GuardrailItem{{Text: "main", Line: 8}, {Text: "", Line: 9}},
		RunOnStop:         []GuardrailCheck{{Line: 5, Problems: []string{"guardrails.run_on_stop: an item needs run"}}},
	}
	w := Workflow{Version: 2, On: map[string][]Entry{PreEdit: {{Line: 6, Keys: []string{"blok"}}}}}
	want := []string{
		"polix.yaml: version must be 1, got 2",
		"polix.yaml:3: guardrails.protected_files: a path glob cannot be empty",
		"polix.yaml:5: guardrails.run_on_stop: an item needs run",
		`polix.yaml:6: unknown key "blok" (when, unless, action)`,
		"polix.yaml:6: the entry needs an action with one of run, inject, block",
		"polix.yaml:7: guardrails.forbidden_commands: an item needs a pattern",
		"polix.yaml:9: guardrails.protected_branches: a branch name cannot be empty",
	}
	if got := g.Problems(w); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := (Guardrails{ProtectedFiles: []GuardrailItem{{Text: ".env", Line: 3}}}).Problems(w); got != nil {
		t.Fatalf("no problems of the shortcuts: %q", got)
	}
}

// An item is valid without problems: a text that is not empty, a pattern, a command to run.
func TestGuardrailsValid(t *testing.T) {
	valid := Guardrails{ProtectedFiles: []GuardrailItem{{Text: ".env", Line: 3}},
		ForbiddenCommands: []GuardrailForbidden{{Pattern: "rm -rf /*", Line: 4}},
		FormatOnEdit:      []GuardrailCheck{{Run: []string{"gofmt"}, Line: 5}}}
	if err := valid.Valid(); err != nil {
		t.Fatal(err)
	}
	for name, g := range map[string]Guardrails{
		"empty glob":     {ProtectedFiles: []GuardrailItem{{Text: " ", Line: 3}}},
		"empty branch":   {ProtectedBranches: []GuardrailItem{{Line: 3}}},
		"no pattern":     {ForbiddenCommands: []GuardrailForbidden{{Line: 4}}},
		"item problems":  {ForbiddenCommands: []GuardrailForbidden{{Pattern: "a", Problems: []string{"x"}}}},
		"no run":         {RunOnStop: []GuardrailCheck{{Line: 5}}},
		"check problems": {FormatOnEdit: []GuardrailCheck{{Run: []string{"x"}, Problems: []string{"x"}}}},
	} {
		if g.Valid() == nil {
			t.Errorf("%s: valid", name)
		}
	}
}

// The name of a protected branch is literal but for `*`, which matches any text, and the expression is anchored.
func TestShortcutsBranchExpr(t *testing.T) {
	g := Guardrails{ProtectedBranches: []GuardrailItem{{Text: "main", Line: 3}, {Text: "release/*", Line: 4},
		{Text: "v1.0", Line: 5}}}
	if got, line := g.branchExpr(); got != `^(?:main|release/.*|v1\.0)$` || line != 3 {
		t.Fatal(got, line)
	}
}
