package format

import (
	"strings"
	"testing"
)

func TestDecideWithoutFile(t *testing.T) {
	if out := Decide(nil, false, false, ""); !out.Missing || out.Write != nil || out.Changed {
		t.Fatalf("got %+v", out)
	}
}

func TestDecideWritesOnlyAChange(t *testing.T) {
	src := []byte("on:\n    stop: []\nversion: 1\n")
	out := Decide(src, true, false, "")
	if !out.Changed || string(out.Write) != "version: 1\non:\n  stop: []\n" || out.Problem != "" {
		t.Fatalf("got %+v %q", out, out.Write)
	}
	if out := Decide(out.Write, true, false, ""); out.Changed || out.Write != nil {
		t.Fatalf("formatted: %+v", out)
	}
}

// AC3: githooks is written after on, pre_commit before commit_msg, and its entries are spaced and ordered as those
// of on, conditions in the order of githooks; a formatted file stays the same.
func TestDecideGithooks(t *testing.T) {
	src := `githooks:
  commit_msg:
    - action:
        run: { message: "m", command: just lint }
      unless: { message: "^{key}: " }
      when: { message: "^x", changed: "*.go", branch: "^[A-Z]+-[0-9]+" }
    - action: { block: "otro" }
  # Before the commit.
  pre_commit:
    - action: { block: { message: "No en main" } }
      when: { branch: "^main$" }
    - action: { run: go vet ./... }
on:
  stop:
    - action: { block: "stop" }
version: 1
`
	want := `version: 1
on:
  stop:
    - action: { block: "stop" }
githooks:
  # Before the commit.
  pre_commit:
    - when: { branch: "^main$" }
      action: { block: { message: "No en main" } }

    - action: { run: go vet ./... }
  commit_msg:
    - when: { branch: "^[A-Z]+-[0-9]+", changed: "*.go", message: "^x" }
      unless: { message: "^{key}: " }
      action:
        run: { command: just lint, message: "m" }

    - action: { block: "otro" }
`
	out := Decide([]byte(src), true, false, "")
	if out.Problem != "" || string(out.Write) != want {
		t.Fatalf("problem %q, got\n%s", out.Problem, out.Write)
	}
	if out := Decide([]byte(want), true, true, ""); out.Changed || out.Problem != "" {
		t.Fatalf("formatted: %+v", out)
	}
}

// With check the change is told but nothing is written.
func TestDecideCheck(t *testing.T) {
	if out := Decide([]byte("version:   1\n"), true, true, ""); !out.Changed || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}

// A polix.yaml that cannot be formatted is a problem, never written.
func TestDecideProblem(t *testing.T) {
	out := Decide([]byte("version: [\n"), true, false, "")
	if out.Problem == "" || out.Write != nil || out.Changed {
		t.Fatalf("got %+v", out)
	}
	out = Decide([]byte("a: &x 1\nb: *x\n"), true, false, "")
	if !strings.Contains(out.Problem, "anchors") || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}

// The command that formats a file names it whenever one was given, quoted when the shell would split it.
func TestFixCommand(t *testing.T) {
	for file, want := range map[string]string{
		"":                    "polix config fmt",
		"polix.yaml":          "polix config fmt polix.yaml",
		"examples/polix.yaml": "polix config fmt examples/polix.yaml",
		"my workflow.yaml":    "polix config fmt 'my workflow.yaml'",
		"it's.yaml":           `polix config fmt 'it'\''s.yaml'`,
	} {
		if got := fixCommand(file); got != want {
			t.Errorf("%q: got %q, want %q", file, got, want)
		}
	}
}
