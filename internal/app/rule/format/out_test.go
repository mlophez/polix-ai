package format

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	ruleload "github.com/mlophez/polix-ai/internal/app/rule/load"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// The adapter of FormatRule is tested against a temporary folder: the real disk, as the port promises.

// Every yaml file of the folder is read by name, retired.yaml included, without other files nor subfolders.
func TestAdapterReadRuleFiles(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"a.yaml": "a", "retired.yaml": "r", "notes.md": "n",
		"sub/b.yaml": "b"} {
		testenv.Write(t, filepath.Join(root, "docs", "rules", filepath.FromSlash(name)), data)
	}
	files, err := Adapter{Root: root}.ReadRuleFiles(context.Background(), "docs/rules")
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	if err != nil || !slices.Equal(names, []string{"a.yaml", "retired.yaml"}) || string(files["a.yaml"]) != "a" {
		t.Fatalf("got %v %v", names, err)
	}
}

// Without the folder there are no files, and no error.
func TestAdapterReadRuleFilesWithoutFolder(t *testing.T) {
	files, err := Adapter{Root: t.TempDir()}.ReadRuleFiles(context.Background(), "ai/rules")
	if files != nil || err != nil {
		t.Fatalf("got %v %v", files, err)
	}
}

// The repeated ids are those of the loader of the rules that cmd/polix builds: two files that share an id, or a rule
// with the id of a retired one. When the loader stops at a file that is not yaml or does not decode, its error is the
// problem, since the repeats of the rules it did not read cannot be ruled out.
func TestAdapterRepeatedIDs(t *testing.T) {
	root := t.TempDir()
	rules := filepath.Join(root, "ai", "rules")
	testenv.Write(t, filepath.Join(rules, "a.yaml"), "rules:\n  - id: A-1\n    description: a\n")
	testenv.Write(t, filepath.Join(rules, "b.yaml"), "rules:\n  - id: A-1\n    description: b\n")
	got := Adapter{Root: root, Rules: ruleload.Command()}.RepeatedIDs(context.Background(), "ai/rules")
	if !slices.Equal(got, []string{"ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"}) {
		t.Fatalf("got %q", got)
	}
	root = t.TempDir()
	rules = filepath.Join(root, "ai", "rules")
	testenv.Write(t, filepath.Join(rules, "a.yaml"), "rules:\n  - id: GOARCH-31\n    description: a\n")
	testenv.Write(t, filepath.Join(rules, "retired.yaml"), "rules:\n  - id: GOARCH-31\n    reason: gone\n")
	got = Adapter{Root: root, Rules: ruleload.Command()}.RepeatedIDs(context.Background(), "ai/rules")
	if !slices.Equal(got, []string{"ai/rules/a.yaml GOARCH-31: id repeated, also in ai/rules/retired.yaml"}) {
		t.Fatalf("a retired id: %q", got)
	}
	testenv.Write(t, filepath.Join(rules, "b.yaml"), "rules: [\n")
	got = Adapter{Root: root, Rules: ruleload.Command()}.RepeatedIDs(context.Background(), "ai/rules")
	if len(got) != 1 || !strings.HasPrefix(got[0], "ai/rules/b.yaml: yaml: ") {
		t.Fatalf("a file that is not yaml: %q", got)
	}
	testenv.Write(t, filepath.Join(rules, "b.yaml"), "rules:\n  - id: GOARCH-31\n    description: b\n")
	testenv.Write(t, filepath.Join(rules, "c.yaml"), "rules:\n  - id: C-1\n    description: c\n    scope: \"*.go\"\n")
	got = Adapter{Root: root, Rules: ruleload.Command()}.RepeatedIDs(context.Background(), "ai/rules")
	if len(got) != 1 || !strings.HasPrefix(got[0], "ai/rules/c.yaml: yaml: ") || !strings.Contains(got[0], "*.go") {
		t.Fatalf("a file that does not decode, after a repeat: %q", got)
	}
}

// polix.yaml is read through the loader that cmd/polix builds: rules.path reaches RulesPath, the problems of a
// workflow that parses are left to polix config check, and a polix.yaml that is not yaml stops. The loader reads
// polix.yaml once, so each case takes a new one.
func TestAdapterLoadWorkflow(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "polix.yaml")
	testenv.Write(t, p, "version: 1\nrules:\n  path: docs/rules\n  run:\n    \"*_test.go\": [go, test, \"{nope}\"]\n")
	wf, err := Adapter{Root: root, Workflow: workflowload.Command()}.LoadWorkflow(context.Background(), "dev")
	if err != nil || wf.RulesPath() != "docs/rules" {
		t.Fatalf("an invalid runner: %q %v", wf.RulesPath(), err)
	}
	testenv.Write(t, p, "version: [\n")
	if _, err := (Adapter{Root: root, Workflow: workflowload.Command()}).LoadWorkflow(context.Background(),
		"dev"); err == nil || !strings.Contains(err.Error(), "polix.yaml") {
		t.Fatalf("a polix.yaml that is not yaml: %v", err)
	}
}

// A rules file is written in its folder, keeping its mode.
func TestAdapterWriteRuleFile(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "ai", "rules", "a.yaml")
	testenv.Write(t, p, "old")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := (Adapter{Root: root}).WriteRuleFile(context.Background(), "ai/rules", "a.yaml", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if got := testenv.Read(t, p); got != "new" {
		t.Fatalf("file: %q", got)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %v", info.Mode().Perm())
	}
}
