package load

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// The validation of the workflow (internal/app/workflow) of the text of polix.yaml: its problems, their lines and
// their order, through the decoding of this use case.

// validate parses src and returns its problems joined, failing the test on an error other than Invalid.
func validate(t *testing.T, src string) string {
	t.Helper()
	w, err := parse(t, src, "dev")
	var invalid workflow.InvalidError
	if err != nil && !errors.As(err, &invalid) {
		t.Fatalf("parse: %v", err)
	}
	if got, want := strings.Join(w.Problems(), "\n"), strings.Join(invalid, "\n"); got != want {
		t.Fatalf("Problems and the error of the loading differ:\n%s\n---\n%s", got, want)
	}
	return strings.Join(w.Problems(), "\n")
}

// Every mistake of this list is reported: a typo never turns into an entry that silently stops applying.
func TestValidRejects(t *testing.T) {
	block := "\n      action: { block: x }\n"
	cases := []struct {
		name, src, want string
	}{
		// AC3: the shape of the action.
		{"no action", "version: 1\non:\n  pre_command:\n    - when: { branch: x }\n", "polix.yaml:4: the entry needs an action with one of run, inject, block"},
		{"empty action", "version: 1\non:\n  pre_command:\n    - action: {}\n", "polix.yaml:4: the entry needs an action with one of run, inject, block"},
		{"two verbs", "version: 1\non:\n  pre_command:\n    - action: { block: x, run: ls }\n", "polix.yaml:4: the action has several verbs (run, block): use one entry per verb"},
		{"unknown key in action", "version: 1\non:\n  pre_command:\n    - action: { block: x, warn: y }\n", `polix.yaml:4: unknown key "warn" in action (run, inject, block)`},
		{"old verb in action", "version: 1\non:\n  pre_command:\n    - action: { deny: x }\n", `polix.yaml:4: unknown key "deny" in action: use block`},
		{"verb without value", "version: 1\non:\n  pre_command:\n    - action: { block: }\n", "polix.yaml:4: block needs a value"},
		{"unknown key in run", "version: 1\non:\n  stop:\n    - action: { run: { command: [ls], rule: x } }\n", `unknown key "rule" in run (command, rules, message, always)`},
		{"unknown key in inject", "version: 1\non:\n  prompt:\n    - action: { inject: { text: x, fiel: y } }\n", `unknown key "fiel" in inject (file, text, rules, always)`},
		{"unknown key in block", "version: 1\non:\n  prompt:\n    - action: { block: { message: x, reason: y } }\n", `unknown key "reason" in block (message, once)`},
		{"once on run", "version: 1\non:\n  stop:\n    - action: { run: { command: ls, once: true } }\n", `unknown key "once" in run`},
		{"once on inject", "version: 1\non:\n  prompt:\n    - action: { inject: { text: x, once: true } }\n", `unknown key "once" in inject`},
		{"always on block", "version: 1\non:\n  prompt:\n    - action: { block: { message: x, always: true } }\n", `unknown key "always" in block`},
		// AC4: the events of each verb.
		{"block on start", "version: 1\non:\n  start:\n    - action: { block: x }\n", "block cannot stop start, which already happened"},
		{"block on post_edit", "version: 1\non:\n  post_edit:\n    - action: { block: x }\n", "block cannot stop post_edit"},
		{"block on post_command", "version: 1\non:\n  post_command:\n    - action: { block: x }\n", "block cannot stop post_command"},
		{"inject on stop", "version: 1\non:\n  stop:\n    - action: { inject: x }\n", "inject does not reach the agent on stop"},
		{"inject on subagent_stop", "version: 1\non:\n  subagent_stop:\n    - action: { inject: { file: x } }\n", "inject does not reach the agent on subagent_stop"},
		// AC5: the long form of each verb.
		{"run without command nor rules", "version: 1\non:\n  stop:\n    - action: { run: { message: x } }\n", "run needs exactly one of command or rules"},
		{"run with command and rules", "version: 1\non:\n  stop:\n    - action: { run: { command: ls, rules: changed } }\n", "run needs exactly one of command or rules"},
		{"empty run", "version: 1\non:\n  stop:\n    - action: { run: \"\" }\n", "run needs exactly one of command or rules"},
		{"inject without source", "version: 1\non:\n  prompt:\n    - action: { inject: { always: true } }\n", "inject needs exactly one of file, text or rules"},
		{"inject with two sources", "version: 1\non:\n  prompt:\n    - action: { inject: { text: x, file: y } }\n", "inject needs exactly one of file, text or rules"},
		{"block without message", "version: 1\non:\n  prompt:\n    - action: { block: { once: true } }\n", "block needs a message"},
		// Placeholders in every string of a verb.
		{"placeholder in a block", "version: 1\non:\n  pre_edit:\n    - action: { block: \"falta {kye}\" }\n", "unknown placeholder {kye}"},
		{"placeholder in the message of a run", "version: 1\non:\n  stop:\n    - action: { run: { command: ls, message: \"{kye}\" } }\n", "unknown placeholder {kye}"},
		{"placeholder in the command of a run", "version: 1\non:\n  stop:\n    - action: { run: \"ls {kye}\" }\n", "unknown placeholder {kye}"},
		{"placeholder in an inject", "version: 1\non:\n  prompt:\n    - action: { inject: { file: \"{kye}.md\" } }\n", "unknown placeholder {kye}"},
		// The shapes of finished.
		{"finished not a string", "version: 1\non:\n  pre_commit:\n    - unless: { finished: { a: b } }" + block, "finished: expected a string or a list of strings"},
		{"finished with a number", "version: 1\non:\n  pre_commit:\n    - unless: { finished: [polix:reviewer, 3] }" + block, "finished: expected strings"},
		{"empty finished", "version: 1\non:\n  pre_commit:\n    - unless: { finished: \"\" }" + block, "finished needs the type of a subagent"},
		// AC10: the measures.
		{"negative measure", "version: 1\non:\n  pre_commit:\n    - when: { risk_score: -1 }" + block, "risk_score: needs a non-negative integer, got -1"},
		{"float measure", "version: 1\non:\n  pre_commit:\n    - when: { risk_score: 2.5 }" + block, "risk_score: needs a non-negative integer, got 2.5"},
		{"text measure", "version: 1\non:\n  pre_commit:\n    - when: { changed_files: many }" + block, "changed_files: needs a non-negative integer, got many"},
		{"map measure", "version: 1\non:\n  pre_commit:\n    - when: { changed_files: {} }" + block, "changed_files: needs a non-negative integer, got map[]"},
		{"invalid measure inside any", "version: 1\non:\n  pre_commit:\n    - when: { any: { changed_lines: -3 } }" + block, "changed_lines: needs a non-negative integer"},
		// AC22: contains inside any.
		{"contains inside any", "version: 1\non:\n  pre_commit:\n    - when: { any: { file: x, contains: y, branch: z } }" + block, "contains cannot go inside any"},
		// The rest of the vocabulary.
		{"unknown key of an entry", "version: 1\non:\n  pre_edit:\n    - wen: { path: x }" + block, `unknown key "wen" (when, unless, action)`},
		{"unknown condition", "version: 1\non:\n  pre_edit:\n    - when: { pth: x }" + block, `unknown condition "pth" (conditions:`},
		{"unknown event", "version: 1\non:\n  pre_push:\n    - action: { block: x }\n", `polix.yaml:3: unknown event "pre_push" (events: start`},
		{"unknown key in vars", "version: 1\nvars:\n  key: { branch: x, brnch: y }\n", `unknown key "brnch" in var key`},
		{"undefined var", "version: 1\nvars:\n  key: { branch: x }\non:\n  prompt:\n    - unless: { var: kye }\n      action: { inject: x }\n", "var kye is not defined"},
		{"placeholder in a condition", "version: 1\non:\n  pre_edit:\n    - unless: { file: \"plans/{kye}.md\" }" + block, "unknown placeholder {kye}"},
		{"reserved var name", "version: 1\nvars:\n  path: { branch: x }\n", "var path: the name is a built-in placeholder"},
		{"reserved measure var name", "version: 1\nvars:\n  risk_score: { branch: x }\n", "var risk_score: the name is a built-in placeholder"},
		{"empty any", "version: 1\non:\n  pre_commit:\n    - when: { any: {} }" + block, "any needs at least one condition"},
		{"any not a map", "version: 1\non:\n  pre_commit:\n    - when: { any: [a] }" + block, "any needs a map of conditions"},
		{"empty weight glob", "version: 1\nrisk:\n  weights:\n    \"\": 1\n", "risk.weights: weight: empty glob"},
		{"negative weight", "version: 1\nrisk:\n  weights:\n    a: -1\n", `risk.weights: weight "a": the factor must be a finite, non-negative number`},
		{"negative max_stop_retries", "version: 1\nmax_stop_retries: -1\n", "max_stop_retries must be positive"},
		{"exit outside post_command", "version: 1\non:\n  pre_command:\n    - when: { exit: 0 }" + block, "exit never holds on pre_command"},
		{"path outside edits", "version: 1\non:\n  stop:\n    - when: { path: x }" + block, "path never holds on stop"},
		{"command inside not outside commands", "version: 1\non:\n  pre_edit:\n    - when: { not: { command: x } }" + block, "command never holds on pre_edit"},
		{"contains not a text", "version: 1\non:\n  pre_edit:\n    - unless: { file: x, contains: [a] }" + block, "contains needs a text"},
		{"contains without file", "version: 1\non:\n  pre_edit:\n    - unless: { contains: a }" + block, "contains only works next to file"},
		{"writes on pre_edit", "version: 1\non:\n  pre_edit:\n    - when: { writes: x }" + block, "writes never holds on pre_edit"},
		{"empty base name", "version: 1\nbase_branch: [main, \"\"]\n", "base_branch: a branch name cannot be empty"},
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

// AC1: every form of the vocabulary is accepted, and constructs that look like mistakes too.
func TestValidAccepts(t *testing.T) {
	src := `version: 1
vars:
  key: { branch: "^([A-Z]+-[0-9]{1,5})" }
on:
  start:
    - action: { inject: "Rama {branch}" }
    - action: { inject: { file: AGENTS.md, always: true } }
    - action: { run: { command: [just, setup], message: "Prepara el entorno", always: false } }
  prompt:
    - unless: { var: key }
      action:
        block: "Crea el ticket antes"
  pre_edit:
    - when: { not: { path: "plans/{key}-*.md" } }
      unless: { file: "plans/{key}-*.md", contains: "Ticket: {key}" }
      action: { block: "Falta el plan de {key} para {path} en {branch}" }
    - action: { inject: { rules: "{path}" } }
  post_edit:
    - when: { path: "*.go" }
      action: { run: "gofmt -l {path}" }
    - when: { path: "checks/**" }
      action: { run: { rules: "{path}", message: "La rule de {path} falla" } }
  pre_command:
    - when: { writes: ["AGENTS.md", "ai/rules/**"], not: { actor: session } }
      action: { block: { message: "Solo una persona cambia {command}" } }
  post_command:
    - when: { command: "just test*", exit: 1 }
      action: { inject: "Los tests fallan" }
  pre_commit:
    - when:
        changed: "*.go"
        any:
          changed_lines: 150
          changed_files: 5
          risk_score: 25
          not: { any: { branch: "^main$" } }
      unless: { finished: polix:reviewer }
      action: { block: "Revisa {changed_lines} lineas en {changed_files} ficheros, riesgo {risk_score}" }
    - when: { changed: ["README.md", "docs/**"] }
      unless: { finished: [polix:reviewer, acme:docs] }
      action: { block: { message: "Revisa la documentación", once: true } }
  subagent_stop:
    - when: { actor: acme:builder }
      action: { run: [just, test] }
  stop:
    - when: { changed: "src/**", not: { branch: "^main$" } }
      unless: { var: key }
      action: { run: { command: just test, always: true } }
    - when: { changed: "src/**", changed_lines: 0 }
      action: { run: { rules: changed } }
rules:
  run:
    "*_test.go": go test {dir} -run ^Test{name}$
risk:
  weights:
    "*_test.go": 0.5
    "internal/**": 3
`
	if got := validate(t, src); got != "" {
		t.Fatalf("unexpected problems:\n%s", got)
	}
}

// AC1: a file with requirements loads valid, with the programs in the workflow in file order.
func TestWorkflowRequirements(t *testing.T) {
	w, err := parse(t, "version: 1\nrequirements: [go, golangci-lint]\nguardrails:\n  protected_files: [AGENTS.md]\n",
		"dev")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(w.Requirements, ",") != "go,golangci-lint" {
		t.Fatalf("requirements: %q", w.Requirements)
	}
	if w, err := parse(t, "version: 1\n", "dev"); err != nil || w.Requirements != nil {
		t.Fatalf("without the key: %q %v", w.Requirements, err)
	}
}

// AC2: each name of the old vocabulary is reported on its line with its new spelling, and an entry whose action
// is of the old vocabulary is not also told it has none.
func TestValidHintsOldNames(t *testing.T) {
	src := `version: 1
on:
  turn_stop:
    - require: "revisa"
  pre_edit:
    - if: { path: x }
      why: "porque"
      mode: warn
      deny: "no"
    - allow: true
    - run: { command: ls }
    - context: { text: x }
    - record: tested
    - unless: { done: tested }
      action: { block: x }
    - when: { changed_lines: { at_least: 150 } }
      action: { block: x }
  session_start:
    - action: { block: x }
`
	got := validate(t, src)
	want := []string{
		`polix.yaml:3: unknown event "turn_stop": use stop`,
		`polix.yaml:6: unknown key "if": use when`,
		`polix.yaml:6: unknown key "why": remove it, or keep the text as a YAML comment`,
		`polix.yaml:6: unknown key "mode": remove it: a warning is an action: { inject: ... }`,
		`polix.yaml:6: unknown key "deny": use action: { block: <message> }`,
		`polix.yaml:10: unknown key "allow": write the exception as a not in when, or in unless`,
		`polix.yaml:11: unknown key "run": move it into action: { run: ... }`,
		`polix.yaml:12: unknown key "context": use action: { inject: ... }`,
		`polix.yaml:13: unknown key "record": remove it: polix observes the end of a subagent by itself, so use ` +
			`finished: <subagent> in when or unless; next to a run, a run that passed is not run again on the same content`,
		`polix.yaml:14: unknown condition "done": use finished: <subagent>`,
		"polix.yaml:16: changed_lines: needs a non-negative integer, got a map: use 150 instead of { at_least: 150 }",
		`polix.yaml:18: unknown event "session_start": use start`,
		"polix.yaml:19: block cannot stop start, which already happened",
	}
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("want %q in:\n%s", w, got)
		}
	}
	if strings.Contains(got, "needs an action") {
		t.Errorf("an entry with an old action is told it has none:\n%s", got)
	}
	// The entries of a renamed event are checked as those of its new name.
	if !strings.Contains(got, `polix.yaml:4: unknown key "require": use action: { block: <message> }`) {
		t.Errorf("the entries of turn_stop:\n%s", got)
	}
	got = validate(t, "version: 1\non:\n  turn_stop:\n    - action: { inject: x }\n")
	if !strings.Contains(got, "inject does not reach the agent on stop") {
		t.Errorf("an entry of turn_stop is checked as one of stop:\n%s", got)
	}
}

// AC2: the conditions under the old key if are checked in the same pass, their hints next to that of if and on its
// line; and an action that only holds verbs of the old vocabulary is not also told it has none.
func TestValidHintsInsideOldIf(t *testing.T) {
	src := `version: 1
on:
  pre_commit:
    - if: { any: { changed_lines: { at_least: 150 } } }
      action: { block: x }
    - if: { done: tested }
      unless: { done: reviewed }
      action: { block: x }
    - if: true
      action: { deny: x }
`
	got := validate(t, src)
	want := strings.Join([]string{
		`polix.yaml:4: unknown key "if": use when`,
		"polix.yaml:4: changed_lines: needs a non-negative integer, got a map: use 150 instead of { at_least: 150 }",
		`polix.yaml:6: unknown key "if": use when`,
		`polix.yaml:6: unknown condition "done": use finished: <subagent>, which holds once that subagent ended on ` +
			`the same files; for the fact of a run, remove it: a run that passed is not run again on the same content`,
		`polix.yaml:6: unknown condition "done": use finished: <subagent>, which holds once that subagent ended on ` +
			`the same files; for the fact of a run, remove it: a run that passed is not run again on the same content`,
		`polix.yaml:9: unknown key "if": use when`,
		`polix.yaml:9: unknown key "deny" in action: use block`,
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The problems follow the file, by line, whatever the order of the events in the map: those of the whole file
// first, then each line in the order it was checked.
func TestProblemsFollowTheFile(t *testing.T) {
	src := `version: 2
on:
  stop:
    - action: { block: "" }
  pre_commit:
    - action: { block: x, run: ls }
  post_edit:
    - action: { block: x }
`
	got := validate(t, src)
	want := strings.Join([]string{
		"polix.yaml: version must be 1, got 2",
		"polix.yaml:4: block needs a message",
		"polix.yaml:6: the action has several verbs (run, block): use one entry per verb",
		"polix.yaml:8: block cannot stop post_edit, which already happened: use inject, or a run whose failure " +
			"reaches the AI",
	}, "\n")
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Renamed tells the problems of a name of the old vocabulary, as Valid reports them, from those of a name this
// polix does not know, which may be an option of a newer one.
func TestRenamed(t *testing.T) {
	src := `version: 1
on:
  turn_stop:
    - if: { done: x }
      action: { deny: x }
    - context: x
  session_start:
    - context: x
  pre_edit:
    - when: { newer: x }
      action: { block: x, newer: y }
      newer: z
  newer_event: []
`
	renamedNames, newer := 0, 0
	for p := range strings.SplitSeq(validate(t, src), "\n") {
		switch {
		case workflow.Problem(p).Renamed():
			renamedNames++
			if strings.Contains(p, "newer") {
				t.Errorf("a new name is not renamed: %s", p)
			}
		case strings.Contains(p, "newer"):
			newer++
		}
	}
	// turn_stop, if, done, deny in action, two context and session_start; newer as a condition, a key of the
	// action, a key and an event.
	if renamedNames != 7 || newer != 4 {
		t.Fatalf("renamed %d, newer %d", renamedNames, newer)
	}
	for _, p := range []string{
		`polix.yaml:3: unknown key "an_option": some text`,
		`polix.yaml:3: unknown key "context" in inject (file, text, rules, always)`,
		`polix.yaml:3: unknown condition "if": use when`,
		"polix.yaml: version must be 1, got 2",
	} {
		if workflow.Problem(p).Renamed() {
			t.Errorf("not a renamed name: %s", p)
		}
	}
}

// The problems of an inject on stop come in the order polix config check reports them: its keys, then the event.
func TestInjectProblemsOrder(t *testing.T) {
	got := validate(t, "version: 1\non:\n  stop:\n    - action: { inject: { text: x, fiel: y } }\n")
	want := "polix.yaml:4: unknown key \"fiel\" in inject (file, text, rules, always)\n" +
		"polix.yaml:4: inject does not reach the agent on stop, which cannot receive context"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The annotated example of the repository, which is also the reference of the skill workflow, is a valid workflow.
func TestExampleIsValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "examples", "polix.yaml"))
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
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "docs", "use-cases.md"))
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
		isWorkflow := false
		for line := range strings.SplitSeq(src, "\n") {
			for _, key := range []string{"version:", "guardrails:", "vars:", "on:"} {
				isWorkflow = isWorkflow || strings.HasPrefix(line, key)
			}
		}
		if !isWorkflow {
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
