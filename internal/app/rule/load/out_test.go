package load

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

// The adapter is tested against a temporary folder: the real disk, as the port promises.

// write writes the files of a test under root, with their folders.
func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for f, data := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A rule is decoded with every field polix reads, its description trimmed and its file.
func TestAdapterReadRules(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"ai/rules/go.yaml": `rules:
  - id: GO-1
    description: >-
      gofmt
    scope: ["**/*.go"]
    deterministic: true
    check: ai/checks/gofmt.sh
    examples: the keys polix does not read are left out
`})
	got, exists, err := Adapter{}.ReadRules(context.Background(), root, "ai/rules")
	if err != nil || !exists {
		t.Fatal(exists, err)
	}
	if len(got) != 1 || got[0].ID != "GO-1" || got[0].Description != "gofmt" || got[0].File != "ai/rules/go.yaml" ||
		!got[0].HasCheck() || got[0].Check != "ai/checks/gofmt.sh" || got[0].Scope[0] != "**/*.go" {
		t.Fatalf("got %+v", got)
	}
}

// The files of a folder of rules, named by their path below it, in the order of their names and each rule in the
// order of its file: retired.yaml, other files and nested folders left out, from a root whose name a glob would read
// as a pattern. Invalid rules are read, not validated.
func TestAdapterReadRulesOfAFolder(t *testing.T) {
	name := "proj[1]*?"
	if runtime.GOOS == "windows" {
		name = "proj[1]" // * and ? are not allowed in names on Windows
	}
	root := filepath.Join(t.TempDir(), name)
	write(t, root, map[string]string{
		"docs/rules/b.yaml":              "rules:\n  - id: B-1\n    description: d\n  - id: A-2\n",
		"docs/rules/a.yaml":              "rules:\n  - id: A-1\n    description: d\n",
		"docs/rules/" + rule.RetiredFile: "rules:\n  - id: R-1\n",
		"docs/rules/notes.md":            "x",
		"docs/rules/sub/nested.yaml":     "rules:\n  - id: N-1\n    description: n\n",
	})
	got, exists, err := Adapter{}.ReadRules(context.Background(), root, "docs/rules")
	if err != nil || !exists {
		t.Fatal(exists, err)
	}
	var read []string
	for _, r := range got {
		read = append(read, r.File+" "+r.ID)
	}
	if want := "docs/rules/a.yaml A-1,docs/rules/b.yaml B-1,docs/rules/b.yaml A-2"; strings.Join(read, ",") != want {
		t.Fatalf("read %v, want %s", read, want)
	}
}

// Without the folder there are no rules, and no error.
func TestAdapterReadRulesWithoutFolder(t *testing.T) {
	got, exists, err := Adapter{}.ReadRules(context.Background(), t.TempDir(), "ai/rules")
	if got != nil || exists || err != nil {
		t.Fatalf("no folder, no rules: %v %v %v", got, exists, err)
	}
}

// Yaml that cannot be decoded stops the reading with the error of the file, after the rules of the files before it.
func TestAdapterReadRulesBrokenYAML(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"ai/rules/a.yaml": "rules:\n  - id: A-1\n",
		"ai/rules/b.yaml": "rules: [\n",
		"ai/rules/c.yaml": "rules:\n  - id: C-1\n    description: d\n",
	})
	got, exists, err := Adapter{}.ReadRules(context.Background(), root, "ai/rules")
	if err == nil || !strings.Contains(err.Error(), "ai/rules/b.yaml") || !exists || len(got) != 1 || got[0].ID != "A-1" {
		t.Fatalf("got %+v %v %v", got, exists, err)
	}
}

// A file of the folder that cannot be read is the error, before any file is decoded.
func TestAdapterReadRulesUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("the permissions of a file do not stop reads on Windows nor for root")
	}
	root := t.TempDir()
	write(t, root, map[string]string{"ai/rules/a.yaml": "rules:\n  - id: A-1\n    description: d\n"})
	p := filepath.Join(root, "ai", "rules", "a.yaml")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	got, exists, err := Adapter{}.ReadRules(context.Background(), root, "ai/rules")
	if err == nil || got != nil || !exists {
		t.Fatalf("got %+v %v %v", got, exists, err)
	}
}
