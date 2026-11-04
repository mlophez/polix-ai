// Package rules loads the rules of ai/rules/*.yaml and runs the check of the deterministic ones.
//
// Polix only reads the fields it needs (id, description, scope, deterministic, check); the rest of the
// rule format belongs to the skills that write and use the rules.
package rules

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/glob"
	"github.com/mlophez/polix-ai/internal/state"
)

// Dir is the folder of the rules, relative to the repository root.
const Dir = "ai/rules"

// retiredFile holds retired rules: they keep their id but never apply.
const retiredFile = "retired.yaml"

// checkTimeout bounds one check script, within the deadline of the caller.
const checkTimeout = 5 * time.Minute

// Rule is one rule of ai/rules/.
type Rule struct {
	ID            string   `yaml:"id"`
	Description   string   `yaml:"description"`
	Scope         []string `yaml:"scope"`
	Deterministic bool     `yaml:"deterministic"`
	Check         string   `yaml:"check"`
	File          string   `yaml:"-"` // file it was read from, relative to the root
}

// Load reads every rule file of root, retired ones excluded, ordered by id.
func Load(root string) ([]Rule, error) {
	// The folder is listed, not globbed: the root may hold [, ], * or ?, which a glob would read as a pattern.
	entries, err := os.ReadDir(filepath.Join(root, Dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []Rule
	for _, e := range entries { // ReadDir sorts by name, as the glob did
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" || e.Name() == retiredFile {
			continue
		}
		f := filepath.Join(root, Dir, e.Name())
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		var doc struct {
			Rules []Rule `yaml:"rules"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.ToSlash(filepath.Join(Dir, filepath.Base(f))), err)
		}
		for i := range doc.Rules {
			doc.Rules[i].File = filepath.ToSlash(filepath.Join(Dir, filepath.Base(f)))
			doc.Rules[i].Description = strings.TrimSpace(doc.Rules[i].Description)
		}
		all = append(all, doc.Rules...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all, nil
}

// Matching returns the rules whose scope matches at least one of the paths; a rule without scope applies to
// the whole project.
func Matching(all []Rule, paths []string) []Rule {
	var out []Rule
	for _, r := range all {
		if len(r.Scope) == 0 && len(paths) > 0 {
			out = append(out, r)
			continue
		}
		for _, p := range paths {
			if glob.AnyPath(r.Scope, p) {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// Validate returns the problems of the rules: missing fields, repeated ids and checks that do not exist.
func Validate(root string, all []Rule) []string {
	var problems []string
	seen := map[string]string{}
	for _, r := range all {
		where := r.File
		if r.ID == "" {
			problems = append(problems, where+": a rule has no id")
			continue
		}
		where += " " + r.ID
		if prev, ok := seen[r.ID]; ok {
			problems = append(problems, fmt.Sprintf("%s: id repeated, also in %s", where, prev))
		}
		seen[r.ID] = r.File
		if r.Description == "" {
			problems = append(problems, where+": no description")
		}
		if r.Deterministic && r.Check != "" {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(r.Check))); err != nil {
				problems = append(problems, fmt.Sprintf("%s: check %s does not exist", where, r.Check))
			}
		}
	}
	return problems
}

// Failure is a rule whose check failed.
type Failure struct {
	Rule   Rule
	Output string // last lines of the output of the check
}

// RunChecks runs the check of every deterministic rule of list that has one, from root, and returns those
// that failed. ctx bounds them all; a check that runs out of time fails.
func RunChecks(ctx context.Context, root string, list []Rule) []Failure {
	var failures []Failure
	for _, r := range list {
		if !r.Deterministic || r.Check == "" {
			continue
		}
		if out, err := runCheck(ctx, root, r.Check); err != nil {
			failures = append(failures, Failure{Rule: r, Output: tail(out+"\n"+err.Error(), 15)})
		}
	}
	return failures
}

// runCheck runs one check script. On Windows a shebang does nothing, so scripts are started through their
// interpreter by extension; elsewhere they run directly, as their shebang says.
func runCheck(ctx context.Context, root, check string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	script := filepath.Join(root, filepath.FromSlash(check))
	argv := []string{script}
	if runtime.GOOS == "windows" {
		interp, err := Interpreter(check, exec.LookPath, fileExists)
		if err != nil {
			return "", err
		}
		argv = append(interp, script)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = root
	// When the deadline kills the script, a grandchild may still hold its output open: stop waiting for it.
	cmd.WaitDelay = WaitDelay
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// Interpreter returns the command line, without the script, that runs a check on Windows, where a shebang does
// nothing: nil for a script that runs by itself. look finds a program on the PATH and exists reports whether a
// file exists; they are parameters so the choice can be tested on any system.
//
// A .sh check runs with the bash of Git for Windows, found next to git, or else with a bash of the PATH outside
// the Windows folder, which is the bash of WSL and runs in another system. A .py check runs with the py launcher,
// then python3, then python, skipping the aliases of the Microsoft Store, which open the Store instead of running.
func Interpreter(check string, look func(string) (string, error), exists func(string) bool) ([]string, error) {
	switch strings.ToLower(winExt(check)) {
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
		return nil, errors.New("no se encuentra bash para ejecutar el check: instala Git for Windows (git y su bash)")
	case ".py":
		for _, c := range []struct {
			name string
			args []string
		}{{"py", []string{"-3"}}, {"python3", nil}, {"python", nil}} {
			if p, err := look(c.name); err == nil && !isStoreAlias(p) {
				return append([]string{p}, c.args...), nil
			}
		}
		return nil, errors.New("no se encuentra python para ejecutar el check: instala Python (py, python3 o python en el PATH)")
	}
	return nil, nil
}

func fileExists(p string) bool {
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

// WaitDelay bounds the wait for the output of a command killed by its deadline, held open by a grandchild that
// outlives it.
const WaitDelay = 5 * time.Second

// MaxLineLen caps each line kept from the output of a check or a command, in bytes.
const MaxLineLen = 500

// tail keeps the last n non-empty lines of s, each cut to MaxLineLen bytes.
func tail(s string, n int) string {
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, state.Cut(l, MaxLineLen))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
