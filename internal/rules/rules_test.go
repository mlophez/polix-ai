package rules

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMatching(t *testing.T) {
	all := []Rule{
		{ID: "ARCH-1", Scope: []string{"src/domain/**"}},
		{ID: "ARCH-2"}, // no scope: the whole project
		{ID: "STYLE-1", Scope: []string{"**/*.tf"}},
	}
	got := Matching(all, []string{"src/domain/order.go"})
	if len(got) != 2 || got[0].ID != "ARCH-1" || got[1].ID != "ARCH-2" {
		t.Fatalf("Matching = %v", got)
	}
	if got := Matching(all, nil); len(got) != 0 {
		t.Fatalf("no paths, no rules: %v", got)
	}
}

// A check killed by its deadline returns even when a grandchild still holds its output open.
func TestRunCheckTimeoutWithGrandchild(t *testing.T) {
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
	if _, err := runCheck(ctx, root, "check.sh"); err == nil {
		t.Fatal("the check should fail on its deadline")
	}
	if d := time.Since(start); d > time.Second+WaitDelay+5*time.Second {
		t.Fatalf("the check returned after %v", d)
	}
}

// tail cuts long lines, so a huge output never reaches the log whole.
func TestTailCutsLines(t *testing.T) {
	out := tail("a\n"+strings.Repeat("x", 10000)+"\n", 15)
	if len(out) > MaxLineLen+2 || !strings.HasPrefix(out, "a\nx") {
		t.Fatalf("tail of %d bytes", len(out))
	}
}

// Rules load from a root whose name a glob would read as a pattern.
func TestLoadFromGlobLikeRoot(t *testing.T) {
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
		"b.yaml":       "rules:\n  - id: B-1\n    description: b\n",
		"a.yaml":       "rules:\n  - id: A-1\n    description: a\n",
		"retired.yaml": "rules:\n  - id: OLD-1\n    description: old\n",
		"notes.md":     "not a rule",
	}
	for f, data := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	all, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != "A-1" || all[1].ID != "B-1" || all[0].File != "ai/rules/a.yaml" {
		t.Fatalf("Load = %+v", all)
	}
	if all, err := Load(t.TempDir()); err != nil || len(all) != 0 {
		t.Fatalf("no folder, no rules: %v %v", all, err)
	}
}

// fakePath is a PATH of fake Windows programs, with the files that exist.
func fakePath(progs map[string]string, files ...string) (func(string) (string, error), func(string) bool) {
	look := func(name string) (string, error) {
		if p, ok := progs[name]; ok {
			return p, nil
		}
		return "", os.ErrNotExist
	}
	exists := func(p string) bool {
		for _, f := range files {
			if f == p {
				return true
			}
		}
		return false
	}
	return look, exists
}

func TestInterpreter(t *testing.T) {
	cases := []struct {
		name  string
		check string
		progs map[string]string
		files []string
		want  string // the command line joined by spaces; empty for none
		err   string // part of the error, when one is expected
	}{
		{"git bash over the WSL one", "checks/a.sh",
			map[string]string{"git": `C:\Program Files\Git\cmd\git.exe`, "bash": `C:\Windows\System32\bash.exe`},
			[]string{`C:\Program Files\Git\bin\bash.exe`}, `C:\Program Files\Git\bin\bash.exe`, ""},
		{"never the WSL bash", "checks/a.sh",
			map[string]string{"bash": `C:\WINDOWS\system32\bash.exe`}, nil, "", "bash"},
		{"another bash on the PATH", "checks/a.sh",
			map[string]string{"bash": `C:\msys64\usr\bin\bash.exe`}, nil, `C:\msys64\usr\bin\bash.exe`, ""},
		{"py launcher first", "checks/a.py",
			map[string]string{"py": `C:\Windows\py.exe`, "python": `C:\Python312\python.exe`}, nil, `C:\Windows\py.exe -3`, ""},
		{"skips the Store alias", "checks/a.py",
			map[string]string{"python3": `C:\Users\u\AppData\Local\Microsoft\WindowsApps\python3.exe`,
				"python": `C:\Python312\python.exe`}, nil, `C:\Python312\python.exe`, ""},
		{"no python", "checks/a.py",
			map[string]string{"python": `C:\Users\u\AppData\Local\Microsoft\WindowsApps\python.exe`}, nil, "", "python"},
		{"runs by itself", "checks/a.exe", nil, nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			look, exists := fakePath(c.progs, c.files...)
			argv, err := Interpreter(c.check, look, exists)
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("err = %v, want one naming %s", err, c.err)
				}
				return
			}
			if err != nil || strings.Join(argv, " ") != c.want {
				t.Fatalf("Interpreter = %q, %v; want %q", argv, err, c.want)
			}
		})
	}
}
