package setup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiagnoseSetupAdapter(t *testing.T) {
	machine(t, true, false)
	ctx := context.Background()
	a := DiagnoseSetupAdapter{OS: runtime.GOOS, Look: exec.LookPath}
	r := gitRepo(t)
	root, gitDir, err := a.OpenRepo(ctx, r)
	if err != nil || root != r {
		t.Fatal(root, err)
	}
	if dir, err := a.WriteState(ctx, gitDir); err != nil || filepath.Dir(dir) != gitDir {
		t.Fatal(dir, err)
	}
	write(t, filepath.Join(r, "checks", "a.sh"), "#!/bin/sh\n")
	if runtime.GOOS != "windows" {
		if err := a.Runnable(ctx, r, "checks/a.sh"); err == nil || err.Error() != "check checks/a.sh is not executable (chmod +x)" {
			t.Fatal(err)
		}
	}
	if a.Runnable(ctx, r, "checks/missing.sh") != nil || !a.Missing(ctx, r, "checks/missing.sh") {
		t.Fatal("a missing check is only missing")
	}
	files, errs := a.ReadFiles(ctx, []string{filepath.Join(r, "checks", "a.sh"), filepath.Join(r, "nope")})
	if len(files) != 1 || len(errs) != 0 {
		t.Fatal(files, errs)
	}
	if out, err := a.ClaudePlugins(ctx); err != nil || strings.TrimSpace(string(out)) != "[]" {
		t.Fatal(string(out), err)
	}
	if _, err := a.LookPath(ctx, "nothing-like-this"); err == nil {
		t.Fatal("look path")
	}
	if !a.SameFile(ctx, r, r) || a.SameFile(ctx, "", r) {
		t.Fatal("same file")
	}
	if err := a.SaveCache(ctx, a.LoadCache(ctx)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := a.ClaudePlugins(ctx); !errors.Is(err, ErrNoCLI) {
		t.Fatal(err)
	}
	if a.CodexUsed(ctx, os.Getenv("HOME"), r) {
		t.Fatal("codex is not used")
	}
}

// The command end to end: in a repository after install, doctor finds the hooks; with the hooks of the user too,
// they run twice; outside a repository only the machine.
func TestDiagnoseSetupCommand(t *testing.T) {
	machine(t, false, false)
	r := gitRepo(t)
	write(t, filepath.Join(r, "polix.yaml"), "version: 1\n")
	if code, _, stderr := runSetup("install", "--yes", "--agent", "claude", "--dir", r); code != 0 {
		t.Fatal(stderr)
	}
	code, stdout, _ := runSetup("doctor", "--offline", "--dir", r)
	if !strings.Contains(stdout, "hooks     registered in") || !strings.Contains(stdout, "polix.yaml is valid") ||
		code != 1 || !strings.Contains(stdout, "polix is not on the PATH") {
		t.Fatalf("doctor after install: %d\n%s", code, stdout)
	}
	data := read(t, filepath.Join(r, ".claude", "settings.json"))
	write(t, filepath.Join(os.Getenv("HOME"), ".claude", "settings.json"), data)
	if _, stdout, _ := runSetup("doctor", "--offline", "--dir", r); !strings.Contains(stdout, "runs polix twice") {
		t.Fatalf("duplicated hooks:\n%s", stdout)
	}
	if code, stdout, _ := runSetup("doctor", "--hook", "--dir", r); code != 0 || !strings.Contains(stdout, "cuéntaselos") {
		t.Fatalf("the hook never fails: %d\n%s", code, stdout)
	}
	if _, stdout, _ := runSetup("doctor", "--offline", "--dir", t.TempDir()); !strings.Contains(stdout, "not inside a git repository") {
		t.Fatalf("outside:\n%s", stdout)
	}
}

// On Windows a check needs its interpreter: a .py check without python on the PATH cannot run, whatever the
// system the test runs on; a missing check is only missing.
func TestDiagnoseSetupAdapterRunnableOnWindows(t *testing.T) {
	r := t.TempDir()
	write(t, filepath.Join(r, "checks", "a.py"), "print(1)\n")
	a := DiagnoseSetupAdapter{OS: "windows", Look: func(string) (string, error) { return "", exec.ErrNotFound }}
	if err := a.Runnable(context.Background(), r, "checks/a.py"); err == nil || !strings.Contains(err.Error(), "python") {
		t.Fatal(err)
	}
	if err := a.Runnable(context.Background(), r, "checks/b.py"); err != nil {
		t.Fatal(err)
	}
}
