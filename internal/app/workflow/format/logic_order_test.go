package format

import (
	"os"
	"path/filepath"
	"testing"
)

// The keys of every section follow the reference, with guardrails and on last; the comments travel with their key,
// and the items of a list and the keys a user names keep their order.
func TestFormat(t *testing.T) {
	src := `# Workflow of the repository.
on:
  stop:
    - action: { block: "stop" }
      when: { changed: "*.go" }
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
    - action: { run: { command: go test ./..., always: true } }
guardrails:
  run_on_stop:
    - changed: ["*.go"]
      run: go vet ./...
  protected_files: [AGENTS.md]
risk:
  weights:
    "z/**": 1
    "a/**": 2
vars:
  z: { branch: "^z" }
  a: { branch: "^a" }
rules: { run: { "*_test.go": [go, test] }, path: ai/rules }
version: 1
`
	want := `# Workflow of the repository.
version: 1
vars:
  z: { branch: "^z" }
  a: { branch: "^a" }
rules: { path: ai/rules, run: { "*_test.go": [go, test] } }
risk:
  weights:
    "z/**": 1
    "a/**": 2
guardrails:
  protected_files: [AGENTS.md]
  run_on_stop:
    - run: go vet ./...
      changed: ["*.go"]
on:
  # Before a commit.
  pre_commit:
    - when:
        changed: ["*.go"]
        any: { changed_lines: 150, risk_score: 25 }
      unless: { finished: polix:reviewer }
      action:
        block:
          once: true
          message: "review"
    - action: { run: { always: true, command: go test ./... } }
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
