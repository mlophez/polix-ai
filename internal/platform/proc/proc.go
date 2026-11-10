// Package proc runs programs: an argv from a folder with a deadline and its combined output, and how a file runs
// as a program on each system (on Windows, where a shebang does nothing, through its interpreter by extension).
package proc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// LookFunc finds a program on the PATH, as exec.LookPath.
type LookFunc func(name string) (string, error)

// ExistsFunc reports whether a file exists, as FileExists.
type ExistsFunc func(path string) bool

// WaitDelay bounds the wait for the output of a command killed by its deadline, held open by a grandchild that
// outlives it.
const WaitDelay = 5 * time.Second

// Run runs argv from dir within timeout (and the deadline of ctx) and returns its stdout and stderr together.
// A command that exits with an error, runs out of time or cannot start returns the error with the output it
// wrote.
func Run(ctx context.Context, dir string, argv []string, timeout time.Duration) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("no command to run")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	// When the deadline kills the command, a grandchild may still hold its output open: stop waiting for it.
	cmd.WaitDelay = WaitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// Program returns the argv that runs the file at path on the system goos: the file itself, or on Windows its
// interpreter followed by the file (see Interpreter). look and exists are parameters so the choice can be tested
// on any system; callers pass exec.LookPath and FileExists.
func Program(goos, path string, look LookFunc, exists ExistsFunc) ([]string, error) {
	if goos != "windows" {
		return []string{path}, nil
	}
	interp, err := Interpreter(path, look, exists)
	if err != nil {
		return nil, err
	}
	return append(interp, path), nil
}

// Interpreter returns the command line, without the file, that runs a file on Windows, where a shebang does
// nothing: nil for a file that runs by itself.
//
// A .sh file runs with the bash of Git for Windows, found next to git, or else with a bash of the PATH outside
// the Windows folder, which is the bash of WSL and runs in another system. A .py file runs with the py launcher,
// then python3, then python, skipping the aliases of the Microsoft Store, which open the Store instead of running.
func Interpreter(file string, look LookFunc, exists ExistsFunc) ([]string, error) {
	switch strings.ToLower(winExt(file)) {
	case ".sh":
		if git, err := look("git"); err == nil {
			dir := winDir(git)
			for _, b := range []string{winDir(dir) + `\bin\bash.exe`, dir + `\bash.exe`} {
				if exists(b) {
					return []string{b}, nil
				}
			}
		}
		if b, err := look("bash"); err == nil && !inWindowsDir(b) {
			return []string{b}, nil
		}
		return nil, errors.New("bash not found to run the check: install Git for Windows (git and its bash)")
	case ".py":
		for _, c := range []struct {
			name string
			args []string
		}{{"py", []string{"-3"}}, {"python3", nil}, {"python", nil}} {
			if p, err := look(c.name); err == nil && !isStoreAlias(p) {
				return append([]string{p}, c.args...), nil
			}
		}
		return nil, errors.New("python not found to run the check: install Python (py, python3 or python on the PATH)")
	}
	return nil, nil
}

// FileExists reports whether p is a file that exists.
func FileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// winDir and winExt split a Windows path on any system, where filepath would not see \ as a separator.
func winDir(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[:i]
	}
	return "."
}

func winExt(p string) string {
	base := p[strings.LastIndexAny(p, `\/`)+1:]
	if i := strings.LastIndex(base, "."); i >= 0 {
		return base[i:]
	}
	return ""
}

func normWin(p string) string { return strings.ToLower(strings.ReplaceAll(p, "/", `\`)) }

// inWindowsDir reports whether a path is below the system folder of Windows, where the bash of WSL lives.
func inWindowsDir(p string) bool {
	n := normWin(p)
	if root := os.Getenv("SystemRoot"); root != "" && strings.HasPrefix(n, normWin(root)+`\`) {
		return true
	}
	return strings.Contains(n, `:\windows\`)
}

// isStoreAlias reports whether a path is an alias of the Microsoft Store, below WindowsApps.
func isStoreAlias(p string) bool { return strings.Contains(normWin(p), `\windowsapps\`) }

// Output runs argv from dir within timeout (none when 0, only the deadline of ctx) and returns what it wrote to
// stdout and to stderr apart. A command that fails returns its error with both outputs.
func Output(ctx context.Context, dir string, argv []string, timeout time.Duration) (string, string, error) {
	if len(argv) == 0 {
		return "", "", errors.New("no command to run")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.WaitDelay = WaitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Stdout runs argv like Output and returns its stdout, also when it fails; the error of a failure carries its
// stderr, trimmed, when it wrote any.
func Stdout(ctx context.Context, dir string, argv []string, timeout time.Duration) (string, error) {
	stdout, stderr, err := Output(ctx, dir, argv, timeout)
	if msg := strings.TrimSpace(stderr); err != nil && msg != "" {
		err = fmt.Errorf("%w: %s", err, msg)
	}
	return stdout, err
}

// shells are the programs that Command resolves on Windows as the interpreters of Interpreter, by the extension
// of the files they run, so a runner never starts the bash of WSL in the Windows folder nor an alias of the Store.
var shells = map[string]string{"bash": ".sh", "sh": ".sh", "python": ".py", "python3": ".py", "py": ".py"}

// Command returns the argv that runs a check on the system goos: argv, the command of its runner, when given, else
// the file at path as a program (Program). On Windows, a runner whose program is bash, sh, python, python3 or py
// (with or without .exe, in any case) runs the program of that name the PATH finds, so an active virtualenv wins,
// unless it is an alias of the Store or, for bash and sh, in the Windows folder (the bash of WSL; the py launcher
// lives there and is kept); then it runs the interpreter
// Interpreter finds for its kind (the bash of Git for Windows, the py launcher, python3 or python). The arguments
// of the runner are kept as they are.
func Command(goos string, argv []string, path string, look LookFunc, exists ExistsFunc) ([]string, error) {
	if len(argv) == 0 {
		return Program(goos, path, look, exists)
	}
	name := strings.TrimSuffix(strings.ToLower(argv[0]), ".exe")
	ext, ok := shells[name]
	if goos != "windows" || !ok {
		return argv, nil
	}
	if p, err := look(argv[0]); err == nil && !isStoreAlias(p) && (ext == ".py" || !inWindowsDir(p)) {
		return append([]string{p}, argv[1:]...), nil
	}
	interp, err := Interpreter("check"+ext, look, exists)
	if err != nil {
		return nil, err
	}
	if name == "py" {
		interp = interp[:1] // the arguments of the runner choose the version, not the -3 of Interpreter
	}
	return append(append([]string{}, interp...), argv[1:]...), nil
}

// RunCheck runs a check from dir within timeout on this system: through argv, the command of its runner, or
// without one the file at path as a program (Command).
func RunCheck(ctx context.Context, dir string, argv []string, path string, timeout time.Duration) (string, error) {
	argv, err := Command(runtime.GOOS, argv, path, exec.LookPath, FileExists)
	if err != nil {
		return "", err
	}
	return Run(ctx, dir, argv, timeout)
}
