package diagnose

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

func TestAdapter(t *testing.T) {
	testenv.Machine(t, true, false)
	ctx := context.Background()
	a := Adapter{OS: runtime.GOOS, Look: exec.LookPath}
	r := testenv.GitRepo(t)
	root, gitDir, err := a.OpenRepo(ctx, r)
	if err != nil || root != r {
		t.Fatal(root, err)
	}
	if dir, err := a.WriteState(ctx, gitDir); err != nil || filepath.Dir(dir) != gitDir {
		t.Fatal(dir, err)
	}
	testenv.Write(t, filepath.Join(r, "checks", "a.sh"), "#!/bin/sh\n")
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
	if _, err := a.ClaudePlugins(ctx); !errors.Is(err, setup.ErrNoCLI) {
		t.Fatal(err)
	}
}

// On Windows a check needs its interpreter: a .py check without python on the PATH cannot run, whatever the
// system the test runs on; a missing check is only missing.
func TestAdapterRunnableOnWindows(t *testing.T) {
	r := t.TempDir()
	testenv.Write(t, filepath.Join(r, "checks", "a.py"), "print(1)\n")
	a := Adapter{OS: "windows", Look: func(string) (string, error) { return "", exec.ErrNotFound }}
	if err := a.Runnable(context.Background(), r, "checks/a.py"); err == nil || !strings.Contains(err.Error(), "python") {
		t.Fatal(err)
	}
	if err := a.Runnable(context.Background(), r, "checks/b.py"); err != nil {
		t.Fatal(err)
	}
}
