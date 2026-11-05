package proc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestHelperProcess is the program the tests of Run start: the test binary itself, with the arguments after
// "--" saying what to do. Run without them, as every test is, it does nothing.
func TestHelperProcess(*testing.T) {
	i := slices.Index(os.Args, "--")
	if i < 0 {
		return
	}
	args := os.Args[i+1:]
	switch args[0] {
	case "echo":
		fmt.Fprintln(os.Stdout, "out")
		fmt.Fprintln(os.Stderr, "err")
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "it failed")
		code, _ := strconv.Atoi(args[1])
		os.Exit(code)
	case "sleep":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	os.Exit(99)
}

// helper is the argv that runs TestHelperProcess with args.
func helper(args ...string) []string {
	return append([]string{os.Args[0], "-test.run=^TestHelperProcess$", "--"}, args...)
}

// Run returns the stdout and stderr of a command together, from the folder it is given.
func TestRunOutput(t *testing.T) {
	out, err := Run(context.Background(), t.TempDir(), helper("echo"), time.Minute)
	if err != nil || !strings.Contains(out, "out") || !strings.Contains(out, "err") {
		t.Fatalf("Run = %q, %v", out, err)
	}
}

// A command that exits with an error returns it, with its exit code and its output.
func TestRunErrorExit(t *testing.T) {
	out, err := Run(context.Background(), t.TempDir(), helper("fail", "3"), time.Minute)
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 3 || !strings.Contains(out, "it failed") {
		t.Fatalf("Run = %q, %v", out, err)
	}
}

// A command that runs out of time fails.
func TestRunTimeout(t *testing.T) {
	start := time.Now()
	if _, err := Run(context.Background(), t.TempDir(), helper("sleep"), 500*time.Millisecond); err == nil {
		t.Fatal("the command should fail on its deadline")
	}
	if d := time.Since(start); d > 500*time.Millisecond+WaitDelay+5*time.Second {
		t.Fatalf("the command returned after %v", d)
	}
}

// A command killed by its deadline returns even when a grandchild still holds its output open.
func TestRunTimeoutWithGrandchild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the command is a sh script")
	}
	root := t.TempDir()
	script := filepath.Join(root, "check.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 60 &\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if _, err := Run(ctx, root, []string{script}, time.Minute); err == nil {
		t.Fatal("the command should fail on its deadline")
	}
	if d := time.Since(start); d > time.Second+WaitDelay+5*time.Second {
		t.Fatalf("the command returned after %v", d)
	}
}

// A command that cannot start, or no command at all, is an error.
func TestRunCannotStart(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := Run(context.Background(), t.TempDir(), []string{missing}, time.Minute); err == nil {
		t.Fatal("a program that does not exist should fail")
	}
	if _, err := Run(context.Background(), t.TempDir(), nil, time.Minute); err == nil {
		t.Fatal("an empty argv should fail")
	}
}

// fakePath is a PATH of fake Windows programs, with the files that exist.
func fakePath(progs map[string]string, files ...string) (LookFunc, ExistsFunc) {
	look := func(name string) (string, error) {
		if p, ok := progs[name]; ok {
			return p, nil
		}
		return "", os.ErrNotExist
	}
	exists := func(p string) bool { return slices.Contains(files, p) }
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
		{"bash next to git", "checks/a.SH",
			map[string]string{"git": `C:\Git\bin\git.exe`},
			[]string{`C:\Git\bin\bash.exe`}, `C:\Git\bin\bash.exe`, ""},
		{"git without its bash falls back to the PATH", "checks/a.sh",
			map[string]string{"git": `C:\Git\cmd\git.exe`, "bash": `C:\msys64\usr\bin\bash.exe`},
			nil, `C:\msys64\usr\bin\bash.exe`, ""},
		{"never the WSL bash", "checks/a.sh",
			map[string]string{"bash": `C:\WINDOWS\system32\bash.exe`}, nil, "", "bash"},
		{"another bash on the PATH", "checks/a.sh",
			map[string]string{"bash": `C:\msys64\usr\bin\bash.exe`}, nil, `C:\msys64\usr\bin\bash.exe`, ""},
		{"no bash", "checks/a.sh", nil, nil, "", "Git for Windows"},
		{"py launcher first", "checks/a.py",
			map[string]string{"py": `C:\Windows\py.exe`, "python": `C:\Python312\python.exe`}, nil, `C:\Windows\py.exe -3`, ""},
		{"python3 before python", "checks/a.py",
			map[string]string{"python3": `C:\Python312\python3.exe`, "python": `C:\Python312\python.exe`}, nil,
			`C:\Python312\python3.exe`, ""},
		{"skips the Store alias", "checks/a.py",
			map[string]string{"python3": `C:\Users\u\AppData\Local\Microsoft\WindowsApps\python3.exe`,
				"python": `C:\Python312\python.exe`}, nil, `C:\Python312\python.exe`, ""},
		{"no python", "checks/a.py",
			map[string]string{"python": `C:\Users\u\AppData\Local\Microsoft\WindowsApps\python.exe`}, nil, "", "python"},
		{"runs by itself", "checks/a.exe", nil, nil, "", ""},
		{"no extension", `checks\a.d\run`, nil, nil, "", ""},
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

// Program runs the file itself outside Windows, and through its interpreter on Windows.
func TestProgram(t *testing.T) {
	look, exists := fakePath(map[string]string{"py": `C:\Windows\py.exe`})
	if argv, err := Program("linux", "/r/checks/a.py", look, exists); err != nil || !slices.Equal(argv, []string{"/r/checks/a.py"}) {
		t.Fatalf("Program on linux = %q, %v", argv, err)
	}
	if argv, err := Program("windows", `C:\r\checks\a.py`, look, exists); err != nil ||
		!slices.Equal(argv, []string{`C:\Windows\py.exe`, "-3", `C:\r\checks\a.py`}) {
		t.Fatalf("Program on windows = %q, %v", argv, err)
	}
	if argv, err := Program("windows", `C:\r\checks\a.exe`, look, exists); err != nil ||
		!slices.Equal(argv, []string{`C:\r\checks\a.exe`}) {
		t.Fatalf("Program of an exe on windows = %q, %v", argv, err)
	}
	if _, err := Program("windows", `C:\r\checks\a.sh`, look, exists); err == nil {
		t.Fatal("Program without bash on windows should fail")
	}
}

func TestFileExists(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !FileExists(f) || FileExists(dir) || FileExists(filepath.Join(dir, "none")) {
		t.Fatal("FileExists is true only for a file that exists")
	}
}

// A bash below the system folder of Windows given by SystemRoot is the bash of WSL, wherever that folder is.
func TestInterpreterSystemRoot(t *testing.T) {
	t.Setenv("SystemRoot", `D:\OS`)
	look, exists := fakePath(map[string]string{"bash": `D:\OS\System32\bash.exe`})
	if argv, err := Interpreter("checks/a.sh", look, exists); err == nil {
		t.Fatalf("Interpreter = %q, want the bash of WSL rejected", argv)
	}
	if !inWindowsDir(`d:/os/system32/bash.exe`) || inWindowsDir(`D:\OSX\bash.exe`) {
		t.Fatal("inWindowsDir follows SystemRoot, case and separators aside")
	}
}

// Output keeps stdout and stderr apart; Stdout folds stderr into the error of a failure.
func TestOutputAndStdout(t *testing.T) {
	out, errOut, err := Output(context.Background(), t.TempDir(), helper("echo"), 0)
	if out != "out\n" || errOut != "err\n" || err != nil {
		t.Fatalf("%q %q %v", out, errOut, err)
	}
	if _, err := Stdout(context.Background(), t.TempDir(), helper("fail", "3"), time.Minute); err == nil ||
		!strings.HasSuffix(err.Error(), ": it failed") {
		t.Fatal(err)
	}
	if _, _, err := Output(context.Background(), "", nil, 0); err == nil {
		t.Fatal("no command")
	}
}

// A runner keeps its argv; on Windows bash, sh, python, python3 and py run the program the PATH names unless it is
// the bash of WSL or an alias of the Store, then the interpreter of their kind; without a runner the file runs as a
// program.
func TestCommand(t *testing.T) {
	paths := map[string]string{
		"git":      `C:\Git\cmd\git.exe`,
		"bash":     `C:\Windows\System32\bash.exe`,
		"BASH.EXE": `C:\Windows\System32\bash.exe`,
		"sh":       `C:\Git\usr\bin\sh.exe`,
		"python":   `C:\repo\.venv\Scripts\python.exe`,
		"python3":  `C:\Users\u\AppData\Local\Microsoft\WindowsApps\python3.exe`,
		"py":       `C:\Windows\py.exe`,
		"py.exe":   `C:\Python\py.exe`,
	}
	look := func(name string) (string, error) {
		if p, ok := paths[name]; ok {
			return p, nil
		}
		return "", errors.New("not found")
	}
	exists := func(p string) bool { return p == `C:\Git\bin\bash.exe` }
	for _, c := range []struct {
		goos string
		argv []string
		want []string
	}{
		{"linux", []string{"bash", "{file}"}, []string{"bash", "{file}"}},
		{"windows", []string{"go", "test"}, []string{"go", "test"}},
		{"windows", []string{"bash", "a.sh"}, []string{`C:\Git\bin\bash.exe`, "a.sh"}},
		{"windows", []string{"BASH.EXE", "a.sh"}, []string{`C:\Git\bin\bash.exe`, "a.sh"}},
		{"windows", []string{"sh", "a.sh"}, []string{`C:\Git\usr\bin\sh.exe`, "a.sh"}},
		{"windows", []string{"python", "a.py"}, []string{`C:\repo\.venv\Scripts\python.exe`, "a.py"}},
		{"windows", []string{"py.exe", "-3.12", "a.py"}, []string{`C:\Python\py.exe`, "-3.12", "a.py"}},
		{"linux", nil, []string{"/r/a.sh"}},
	} {
		got, err := Command(c.goos, c.argv, "/r/a.sh", look, exists)
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s %v: got %v %v, want %v", c.goos, c.argv, got, err, c.want)
		}
	}
	// python3 is a Store alias: it falls back to the interpreter lookup, which finds the py launcher.
	if got, err := Command("windows", []string{"python3", "a.py"}, "", look, exists); err != nil ||
		!slices.Equal(got, []string{`C:\Windows\py.exe`, "-3", "a.py"}) {
		t.Fatal(got, err)
	}
	// The py launcher lives in the Windows folder and is kept, with the version flag of the runner once.
	if got, err := Command("windows", []string{"py", "-3.12", "a.py"}, "", look, exists); err != nil ||
		!slices.Equal(got, []string{`C:\Windows\py.exe`, "-3.12", "a.py"}) {
		t.Fatal(got, err)
	}
	// A py that is a Store alias falls back to the interpreter lookup, without its -3.
	paths["py"] = `C:\Users\u\AppData\Local\Microsoft\WindowsApps\py.exe`
	if got, err := Command("windows", []string{"py", "-3.12", "a.py"}, "", look, exists); err != nil ||
		!slices.Equal(got, []string{`C:\repo\.venv\Scripts\python.exe`, "-3.12", "a.py"}) {
		t.Fatal(got, err)
	}
	// Only Store aliases: no python.
	paths["python"] = paths["python3"]
	if _, err := Command("windows", []string{"python3", "a.py"}, "", look, exists); err == nil {
		t.Fatal("only Store aliases: no python")
	}
	if out, err := RunCheck(context.Background(), t.TempDir(), helper("echo"), "", time.Minute); err != nil ||
		!strings.Contains(out, "out") {
		t.Fatal(out, err)
	}
}
