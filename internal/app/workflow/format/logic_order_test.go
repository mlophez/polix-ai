package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The keys of every section follow the reference, with guardrails and on last; the comments travel with their key,
// and the items of a list and the keys a user names keep their order. CUJ-09, AC-08: guardrails, written in any
// order, comes out with the protections first and then the lifecycle shortcuts (LIT-09), each with its comment.
func TestFormat(t *testing.T) {
	src := `# Workflow of the repository.
on:
  stop:
    - action: { block: "stop" }
      when: { changed: "*.go" }
  post_commit:
    - action: { notify: "Commit done" }
  # Before a commit.
  pre_commit:
    - unless: { finished: polix:reviewer }
      action:
        block:
          message: "review"
          once: true
      when:
        any: { risk_score: 25, changed_lines: 150 }
        changed: ["*.go"]
        actor: session
    - action: { run: { command: go test ./..., always: true } }
guardrails:
  # Every commit, whoever makes it.
  check_on_save:
    - message: "lint"
      changed: ["*.go"]
      run: golangci-lint run ./...
    - run: go test ./...
  # Nobody runs these.
  forbidden_commands:
    - pattern: "git push --force*"
  exec_on_start:
    - message: "uv failed:"
      run: uv sync --locked
    - run: just setup
  protected_branches: [main]
  protected_files: [AGENTS.md]
  check_on_stop:
    - changed: ["*.go"]
      run: go vet ./...
  format_on_save:
    - changed: "*.ts"
      message: "eslint:"
      run: npx eslint --fix
  format_on_edit:
    - path: "*.go"
      run: gofmt -w {path}
risk:
  weights:
    "z/**": 1
    "a/**": 2
vars:
  z: { branch: "^z" }
  a: { branch: "^a" }
rules: { run: { "*_test.go": [go, test] } }
version: 1
`
	want := `# Workflow of the repository.
version: 1
vars:
  z: { branch: "^z" }
  a: { branch: "^a" }
rules: { run: { "*_test.go": [go, test] } }
risk:
  weights:
    "z/**": 1
    "a/**": 2
guardrails:
  protected_files: [AGENTS.md]
  protected_branches: [main]
  # Nobody runs these.
  forbidden_commands:
    - pattern: "git push --force*"
  exec_on_start:
    - run: uv sync --locked
      message: "uv failed:"

    - run: just setup
  format_on_edit:
    - run: gofmt -w {path}
      path: "*.go"
  format_on_save:
    - run: npx eslint --fix
      changed: "*.ts"
      message: "eslint:"
  check_on_stop:
    - run: go vet ./...
      changed: ["*.go"]
  # Every commit, whoever makes it.
  check_on_save:
    - run: golangci-lint run ./...
      changed: ["*.go"]
      message: "lint"

    - run: go test ./...
on:
  # Before a commit.
  pre_commit:
    - when:
        actor: session
        changed: ["*.go"]
        any: { changed_lines: 150, risk_score: 25 }
      unless: { finished: polix:reviewer }
      action:
        block:
          once: true
          message: "review"

    - action: { run: { command: go test ./..., always: true } }
  post_commit:
    - action: { notify: "Commit done" }
  stop:
    - when: { changed: "*.go" }
      action: { block: "stop" }
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := formatData(got); err != nil || string(again) != want {
		t.Fatalf("not idempotent: %v\n%s", err, again)
	}
	lit09 := "builtin, protected_files, protected_branches, forbidden_commands, exec_on_start, format_on_edit, " +
		"format_on_save, check_on_stop, check_on_save"
	if keys := strings.Join(order([]string{"guardrails"}), ", "); keys != lit09 {
		t.Fatalf("the order of guardrails: %s", keys)
	}
	if strings.Contains(string(got), "polix install") {
		t.Fatal("CUJ-03: the entry check_on_save expands into in start is never written into the file")
	}
}

// AC2: requirements, written after on, goes right before guardrails, its programs and its comment kept in order.
func TestFormatOrderRequirements(t *testing.T) {
	src := `version: 1
guardrails:
  protected_files: [AGENTS.md]
on:
  stop:
    - action: { block: "stop" }
# The programs the hooks run.
requirements: [golangci-lint, go]
risk:
  weights:
    "a/**": 2
`
	want := `version: 1
risk:
  weights:
    "a/**": 2
# The programs the hooks run.
requirements: [golangci-lint, go]
guardrails:
  protected_files: [AGENTS.md]
on:
  stop:
    - action: { block: "stop" }
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := formatData(got); err != nil || string(again) != want {
		t.Fatalf("not idempotent: %v\n%s", err, again)
	}
}

// CUJ-09, AC-06: path goes right after version, before requires and rules, with its comment.
func TestFormatOrderPath(t *testing.T) {
	src := `rules:
  run: { "*_test.go": [go, test] }
requires: 0.3.0
# The folder of the AI.
path: docs/ai
version: 1
`
	want := `version: 1
# The folder of the AI.
path: docs/ai
requires: 0.3.0
rules:
  run: { "*_test.go": [go, test] }
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	got, err = formatData([]byte("rules:\n  run: { x: [y] }\npath: docs/ai\nversion: 1\n"))
	if err != nil || !strings.HasPrefix(string(got), "version: 1\npath: docs/ai\nrules:\n") {
		t.Fatalf("CUJ-09: %v\n%s", err, got)
	}
}

// CUJ-14, AC-09: builtin, written after check_on_save with its keys in reverse order, goes first in guardrails with
// its keys in the order of workflow.BuiltinKeys and its comment, in the block or the flow form, and a second run
// changes nothing.
func TestFormatBuiltin(t *testing.T) {
	src := `version: 1
guardrails:
  check_on_save:
    - run: go test ./...
  # What polix applies by itself.
  builtin:
    check_polix_files: false
    format_polix_files: true
    forbid_git_destructive_commands: false
    forbid_commit_no_verify: true
  protected_files: [AGENTS.md]
`
	want := `version: 1
guardrails:
  # What polix applies by itself.
  builtin:
    forbid_commit_no_verify: true
    forbid_git_destructive_commands: false
    format_polix_files: true
    check_polix_files: false
  protected_files: [AGENTS.md]
  check_on_save:
    - run: go test ./...
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := formatData(got); err != nil || string(again) != want {
		t.Fatalf("not idempotent: %v\n%s", err, again)
	}
	flow := "version: 1\nguardrails:\n  protected_files: [x]\n  builtin: { check_polix_files: false, forbid_commit_no_verify: true }\n"
	got, err = formatData([]byte(flow))
	if want := "version: 1\nguardrails:\n  builtin: { forbid_commit_no_verify: true, check_polix_files: false }\n" +
		"  protected_files: [x]\n"; err != nil || string(got) != want {
		t.Fatalf("flow:\n%s\nwant:\n%s", got, want)
	}
}

// CUJ-14, AC-09: also_on_save, written before run, goes last in an item of check_on_stop, after wait_agents, and a
// second run changes nothing.
func TestFormatAlsoOnSave(t *testing.T) {
	src := `version: 1
guardrails:
  check_on_stop:
    - also_on_save: true
      wait_agents: false
      run: go vet ./...
      message: Fix go vet
      changed: "*.go"
`
	want := `version: 1
guardrails:
  check_on_stop:
    - run: go vet ./...
      changed: "*.go"
      message: Fix go vet
      wait_agents: false
      also_on_save: true
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := formatData(got); err != nil || string(again) != want {
		t.Fatalf("not idempotent: %v\n%s", err, again)
	}
}

// CUJ-14, AC-09: stop_retries, written after guardrails, goes where max_stop_retries went, right after base_branch,
// and a second run changes nothing.
func TestFormatStopRetries(t *testing.T) {
	src := `version: 1
vars:
  key: { branch: "^x" }
guardrails:
  protected_files: [AGENTS.md]
# The loop guard.
stop_retries: 2
base_branch: main
`
	want := `version: 1
base_branch: main
# The loop guard.
stop_retries: 2
vars:
  key: { branch: "^x" }
guardrails:
  protected_files: [AGENTS.md]
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := formatData(got); err != nil || string(again) != want {
		t.Fatalf("not idempotent: %v\n%s", err, again)
	}
}

// AC9: the keys of the long form of notify follow workflow.NotifyKeys, always before message, and notify comes last
// among the verbs of an action.
func TestFormatOrderNotify(t *testing.T) {
	src := `version: 1
on:
  prompt:
    - action:
        notify:
          message: "Rama {branch}"
          always: true
    - action: { notify: { message: x, always: false } }
`
	want := `version: 1
on:
  prompt:
    - action:
        notify:
          always: true
          message: "Rama {branch}"

    - action: { notify: { always: false, message: x } }
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if got := order([]string{"on", "prompt", "-", "action"}); got[len(got)-1] != "notify" {
		t.Fatalf("the verbs: %q", got)
	}
}

// The entries of an event and the items of the shortcuts that take maps get one blank line between them, above the
// comments of the later one, unless both are written on one line; the lists of texts and the lists inside an entry
// stay together, and the keys of a long run follow workflow.RunKeys, always after the message.
func TestFormatSpaced(t *testing.T) {
	src := `version: 1
guardrails:
  protected_files:
    - AGENTS.md
    - CLAUDE.md
  forbidden_commands:
    - pattern: "git reset --hard*"
    - pattern: "git clean -f*"
  check_on_stop:
    - run: go vet ./...
    - run: go test ./...
      changed: ["*.go"]
on:
  start:
    - action:
        run:
          always: true
          message: "failed"
          command: git config --local core.hooksPath .githooks
    # The second one.
    - action: { inject: x }
  pre_commit:
    - when:
        any:
          - changed_lines: 150
          - risk_score: 25
      action: { block: "review" }
`
	want := `version: 1
guardrails:
  protected_files:
    - AGENTS.md
    - CLAUDE.md
  forbidden_commands:
    - pattern: "git reset --hard*"
    - pattern: "git clean -f*"
  check_on_stop:
    - run: go vet ./...

    - run: go test ./...
      changed: ["*.go"]
on:
  start:
    - action:
        run:
          command: git config --local core.hooksPath .githooks
          message: "failed"
          always: true

    # The second one.
    - action: { inject: x }
  pre_commit:
    - when:
        any:
          - changed_lines: 150
          - risk_score: 25
      action: { block: "review" }
`
	got, err := formatData([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if again, err := formatData(got); err != nil || string(again) != want {
		t.Fatalf("not idempotent: %v\n%s", err, again)
	}
}

// The workflows of the repository, formatted in the order of the reference, keep their data and are stable.
func TestFormatRepositoryWorkflows(t *testing.T) {
	for _, name := range []string{"polix.yaml", "examples/polix.yaml"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "..", "..", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		once, err := formatData(src)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if twice, err := formatData(once); err != nil || string(twice) != string(once) {
			t.Fatalf("%s: not stable: %v", name, err)
		}
	}
}

// Broken YAML cannot be formatted; a workflow with problems can.
func TestFormatErrors(t *testing.T) {
	if _, err := formatData([]byte("on: [\n")); err == nil {
		t.Fatal("broken yaml")
	}
	if _, err := formatData([]byte("unknown: 1\nversion: 1\n")); err != nil {
		t.Fatal(err)
	}
}
