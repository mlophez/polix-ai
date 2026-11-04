package workflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validate parses src and returns its problems joined, failing the test on a parse error.
func validate(t *testing.T, src string) string {
	t.Helper()
	w, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return strings.Join(Validate(w), "\n")
}

// Every mistake of this list is reported: a typo never turns into an entry that silently stops applying.
func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"allow false", "version: 1\non:\n  pre_command:\n    - allow: false\n", "allow only takes true"},
		{"unknown key in context", "version: 1\non:\n  prompt:\n    - context: { text: x, fiel: y }\n", `unknown key "fiel" in context`},
		{"unknown key in run", "version: 1\non:\n  stop:\n    - run: { command: [ls], rule: x }\n", `unknown key "rule" in run`},
		{"empty record in run", "version: 1\non:\n  stop:\n    - run: { command: [ls], record: \"\" }\n", "record in run needs the name of the fact"},
		{"placeholder in run record", "version: 1\non:\n  stop:\n    - run: { command: [ls], record: \"{kye}\" }\n", "unknown placeholder {kye}"},
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
		{"allow on post_edit", "version: 1\non:\n  post_edit:\n    - allow: true\n", "allow cannot let post_edit through"},
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
func TestValidateAccepts(t *testing.T) {
	src := `version: 1
vars:
  key: { branch: "^([A-Z]+-[0-9]{1,5})" }
on:
  pre_edit:
    - if: { path: "plans/{key}-*.md" }
      allow: true
    - unless: { file: "plans/{key}-*.md", contains: "Ticket: {key}" }
      deny: "Falta el plan de {key} para {path} en {branch}"
  post_command:
    - if: { command: "just test*", exit: 0 }
      record: tested
  stop:
    - if: { changed: "src/**", not: { branch: "^main$" } }
      unless: { done: tested, var: key }
      require: "Ejecuta just test"
`
	if got := validate(t, src); got != "" {
		t.Fatalf("unexpected problems:\n%s", got)
	}
}

// requires is read before the rest of the file, so a polix older than the workflow asks to be updated instead of
// reporting the options it does not know; a development build is never too old.
func TestLoadRequires(t *testing.T) {
	root := t.TempDir()
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(root, File), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("version: 1\nrequires: 0.3.0\nan_option_of_0_3: true\n")
	_, err := Load(root, "0.2.1")
	var old *TooOld
	if !errors.As(err, &old) || old.Requires != "0.3.0" || old.Running != "0.2.1" {
		t.Fatalf("an older polix: %v", err)
	}
	if _, err := Load(root, "dev"); err == nil || errors.As(err, &old) {
		t.Fatalf("a dev build parses the file and reports what it does not know: %v", err)
	}
	if _, err := Load(root, "0.3.0"); err == nil || errors.As(err, &old) {
		t.Fatalf("the required version parses the file: %v", err)
	}

	write("version: 1\nrequires: v0.2.0\n")
	w, err := Load(root, "0.2.0")
	if err != nil || w.Requires != "v0.2.0" || len(Validate(w)) != 0 {
		t.Fatalf("a polix as new as required: %v %+v %v", err, w, Validate(w))
	}
	write("version: 1\nrequires: 0.2\n")
	if w, err := Load(root, "0.2.0"); err != nil || len(Validate(w)) != 1 {
		t.Fatalf("requires must be a full version: %v", err)
	}
}
