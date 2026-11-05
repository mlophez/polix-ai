package rule

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/pkg/git"
)

// The adapter is tested against a temporary repository: real disk and processes, as the port promises.

// Rules load from a root whose name a glob would read as a pattern, ordered by id and without the retired ones.
func TestCheckRuleAdapterLoadRulesFromGlobLikeRoot(t *testing.T) {
	name := "proj[1]*?"
	if runtime.GOOS == "windows" {
		name = "proj[1]" // * and ? are not allowed in names on Windows
	}
	root := filepath.Join(t.TempDir(), name)
	dir := filepath.Join(root, filepath.FromSlash(Dir))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"b.yaml":    "rules:\n  - id: B-1\n    description: b\n",
		"a.yaml":    "rules:\n  - id: A-1\n    description: a\n",
		RetiredFile: "rules:\n  - id: OLD-1\n    description: old\n",
		"notes.md":  "not a rule",
	}
	for f, data := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a := CheckRuleAdapter{Repo: &git.Repo{Root: root}}
	all, err := a.LoadRules(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != "A-1" || all[1].ID != "B-1" || all[0].File != "ai/rules/a.yaml" {
		t.Fatalf("LoadRules = %+v", all)
	}
	a = CheckRuleAdapter{Repo: &git.Repo{Root: t.TempDir()}}
	if all, err := a.LoadRules(context.Background()); err != nil || len(all) != 0 {
		t.Fatalf("no folder, no rules: %v %v", all, err)
	}
}

// A check killed by its deadline returns even when a grandchild still holds its output open.
func TestCheckRuleAdapterRunScriptTimeoutWithGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the check is a sh script")
	}
	root := t.TempDir()
	script := "#!/bin/sh\nsleep 60 &\nsleep 60\n"
	if err := os.WriteFile(filepath.Join(root, "check.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	a := CheckRuleAdapter{Repo: &git.Repo{Root: root}}
	if _, err := a.RunScript(ctx, "check.sh"); err == nil {
		t.Fatal("the check should fail on its deadline")
	}
	if d := time.Since(start); d > time.Second+checkRuleWaitDelay+5*time.Second {
		t.Fatalf("the check returned after %v", d)
	}
}
