package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validate parses src and returns its problems joined, failing the test on an error other than Invalid.
func validate(t *testing.T, src string) string {
	t.Helper()
	w, err := Parse([]byte(src), "dev")
	var invalid Invalid
	if err != nil && !errors.As(err, &invalid) {
		t.Fatalf("parse: %v", err)
	}
	if got, want := strings.Join(w.Problems(), "\n"), strings.Join(invalid, "\n"); got != want {
		t.Fatalf("Problems and the error of Parse differ:\n%s\n---\n%s", got, want)
	}
	return strings.Join(w.Problems(), "\n")
}

// Every mistake of this list is reported: a typo never turns into an entry that silently stops applying.
func TestValidRejects(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"allow false", "version: 1\non:\n  pre_command:\n    - allow: false\n", "allow only takes true"},
		{"unknown key in context", "version: 1\non:\n  prompt:\n    - context: { text: x, fiel: y }\n", `unknown key "fiel" in context`},
		{"unknown key in run", "version: 1\non:\n  stop:\n    - run: { command: [ls], rule: x }\n", `unknown key "rule" in run`},
		{"record inside run", "version: 1\non:\n  stop:\n    - run: { command: [ls], record: tested }\n", "record goes next to run"},
		{"empty record next to run", "version: 1\non:\n  stop:\n    - run: { command: [ls] }\n      record: \"\"\n", "record needs the name of the fact"},
		{"placeholder in a record next to require", "version: 1\non:\n  stop:\n    - require: x\n      record: \"{kye}\"\n", "unknown placeholder {kye}"},
		{"unknown key in vars", "version: 1\nvars:\n  key: { branch: x, brnch: y }\n", `unknown key "brnch" in var key`},
		{"undefined var", "version: 1\nvars:\n  key: { branch: x }\non:\n  prompt:\n    - unless: { var: kye }\n      context: { text: x }\n", "var kye is not defined"},
		{"undefined placeholder", "version: 1\non:\n  pre_edit:\n    - deny: \"falta {kye}\"\n", "unknown placeholder {kye}"},
		{"placeholder in a condition", "version: 1\non:\n  pre_edit:\n    - unless: { file: \"plans/{kye}.md\" }\n      deny: x\n", "unknown placeholder {kye}"},
		{"reserved var name", "version: 1\nvars:\n  path: { branch: x }\n", "var path: the name is a built-in placeholder"},
		{"negative max_stop_blocks", "version: 1\nmax_stop_blocks: -1\n", "max_stop_blocks must be positive"},
		{"context on stop", "version: 1\non:\n  stop:\n    - context: { text: x }\n", "context does not reach the agent on stop"},
		{"exit outside post_command", "version: 1\non:\n  pre_command:\n    - if: { exit: 0 }\n      deny: x\n", "exit never holds on pre_command"},
		{"path outside edits", "version: 1\non:\n  stop:\n    - if: { path: x }\n      require: x\n", "path never holds on stop"},
		{"command inside not outside commands", "version: 1\non:\n  pre_edit:\n    - if: { not: { command: x } }\n      deny: x\n", "command never holds on pre_edit"},
		{"contains not a text", "version: 1\non:\n  pre_edit:\n    - unless: { file: x, contains: [a] }\n      deny: x\n", "contains needs a text"},
		{"writes on pre_edit", "version: 1\non:\n  pre_edit:\n    - if: { writes: x }\n      deny: x\n", "writes never holds on pre_edit"},
		{"empty protected glob", "version: 1\nprotected: [\".env\", \"\"]\n", "protected: a path glob cannot be empty"},
		{"empty base name", "version: 1\nbase: [main, \"\"]\n", "base: a branch name cannot be empty"},
		{"allow on post_edit", "version: 1\non:\n  post_edit:\n    - allow: true\n", "allow cannot let post_edit through"},
		{"empty runner", "version: 1\nrules:\n  run:\n    \"*_test.go\": []\n", `polix.yaml:4: rules.run "*_test.go": the command is empty`},
		{"unknown key in rules", "version: 1\nrules:\n  runs: {}\n", `unknown key "runs" in rules (path, run)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := validate(t, c.src); !strings.Contains(got, c.want) {
				t.Fatalf("want a problem with %q, got:\n%s", c.want, got)
			}
		})
	}
}

// Valid constructs that look like mistakes are accepted.
func TestValidAccepts(t *testing.T) {
	src := `version: 1
vars:
  key: { branch: "^([A-Z]+-[0-9]{1,5})" }
on:
  pre_edit:
    - if: { path: "plans/{key}-*.md" }
      allow: true
    - unless: { file: "plans/{key}-*.md", contains: "Ticket: {key}" }
      deny: "Falta el plan de {key} para {path} en {branch}"
  pre_command:
    - if: { writes: ["AGENTS.md", "ai/rules/**"], not: { actor: session } }
      deny: "Solo una persona cambia {command}"
  post_command:
    - if: { command: "just test*", exit: 0 }
      record: tested
  stop:
    - if: { changed: "src/**", not: { branch: "^main$" } }
      unless: { done: tested, var: key }
      require: "Ejecuta just test"
rules:
  run:
    "*_test.go": go test {dir} -run ^Test{name}$
`
	if got := validate(t, src); got != "" {
		t.Fatalf("unexpected problems:\n%s", got)
	}
}

// The problems of a context on stop come in the order polix check reports them: its keys, then the event.
func TestContextProblemsOrder(t *testing.T) {
	got := validate(t, "version: 1\non:\n  stop:\n    - context: { text: x, fiel: y }\n")
	want := "polix.yaml:4: unknown key \"fiel\" in context (file, text, rules)\n" +
		"polix.yaml:4: context does not reach the agent on stop, which cannot receive context"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Valid joins every problem in one Invalid error, whose list is that of Problems.
func TestValidJoinsProblems(t *testing.T) {
	w := Workflow{Version: 2, MaxStopBlocks: -1}
	err := w.Valid()
	var invalid Invalid
	if !errors.As(err, &invalid) || len(invalid) != 2 || err.Error() != strings.Join(w.Problems(), "\n") {
		t.Fatalf("Valid: %v", err)
	}
	if err := (Workflow{Version: 1}).Valid(); err != nil {
		t.Fatalf("a minimal workflow: %v", err)
	}
}

// A variable outside a workflow reports its own problems, without a name.
func TestVarValid(t *testing.T) {
	cases := map[string]Var{
		"var needs branch":                {},
		"var: error parsing regexp":       {Branch: "("},
		`unknown key "x" in var (branch)`: {Branch: "x", Keys: []string{"branch", "x"}},
	}
	for want, v := range cases {
		if err := v.Valid(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: want %q, got %v", v, want, err)
		}
	}
	if err := (Var{Branch: "^main$"}).Valid(); err != nil {
		t.Fatal(err)
	}
}

// The annotated example of the repository, which is also the reference of the skill workflow, is a valid workflow.
func TestExampleIsValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "polix.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := validate(t, string(data)); problems != "" {
		t.Fatalf("examples/polix.yaml:\n%s", problems)
	}
}

// Every recipe of docs/use-cases.md is a valid workflow once `version: 1` is added. The yaml blocks that are not a
// workflow (a rule of ai/rules/, a step of CI) have no top-level key of polix.yaml and are skipped.
func TestUseCasesAreValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "use-cases.md"))
	if err != nil {
		t.Fatal(err)
	}
	blocks := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "```yaml\n")
	checked := 0
	for i, b := range blocks[1:] {
		src, _, ok := strings.Cut(b, "```")
		if !ok {
			t.Fatalf("block %d: not closed", i+1)
		}
		workflow := false
		for line := range strings.SplitSeq(src, "\n") {
			for _, key := range []string{"version:", "protected:", "vars:", "on:"} {
				workflow = workflow || strings.HasPrefix(line, key)
			}
		}
		if !workflow {
			continue
		}
		if !strings.Contains("\n"+src, "\nversion:") {
			src = "version: 1\n" + src
		}
		if problems := validate(t, src); problems != "" {
			t.Errorf("block %d of docs/use-cases.md:\n%s\n%s", i+1, src, problems)
		}
		checked++
	}
	if checked < 20 {
		t.Fatalf("only %d recipes found in docs/use-cases.md", checked)
	}
}
