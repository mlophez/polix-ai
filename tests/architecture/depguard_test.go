package architecture

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

// lintIssue is one finding of golangci-lint in its JSON output.
type lintIssue struct {
	FromLinter string
	Text       string
	Pos        struct {
		Filename string
		Line     int
	}
}

// lintRun is the run of depguard on one root, once per test binary.
type lintRun struct {
	once  sync.Once
	found []string // the findings, as path:line: text, the path relative to the root with slashes
	err   error
}

// depguardList is a list of the settings of depguard in .golangci.yml: the packages it allows and the ones it
// denies. An entry is $gostd, a package with $ at its end (that package alone), or a prefix (the package and every
// package below it).
type depguardList struct {
	Allow []string       `yaml:"allow"`
	Deny  []depguardDeny `yaml:"deny"`
}

// depguardDeny is an entry of the packages a list of depguard denies.
type depguardDeny struct {
	Pkg string `yaml:"pkg"`
}

// depguardRoot is a module that breaks each list of depguard once, so each test of a rule that depguard decides
// proves that it detects it. Unlike testdata/bad it compiles, since golangci-lint loads its types.
const depguardRoot = "testdata/depguard"

// installLint says how to get golangci-lint when it is missing.
const installLint = "install golangci-lint v2.15.0, the version that .github/workflows/ci.yml pins " +
	"(https://golangci-lint.run/docs/welcome/install/), and put it on the PATH"

// linted caches the runs of depguard by root, shared by the tests of every rule it decides.
var linted sync.Map

// checkDepguard runs depguard, through golangci-lint, on the module under check, where the lists of rule must find
// nothing, and on testdata/depguard, where every string of want must appear in some finding of those lists and no
// test file is reported. The lists of rule are the one named rule and those named rule-<part>. Without
// golangci-lint the test fails, never skips.
func checkDepguard(t *testing.T, rule string, want ...string) {
	t.Helper()
	t.Run("module", func(t *testing.T) {
		for _, f := range mustLint(t, *root, rule) {
			t.Error(f)
		}
	})
	t.Run("detects", func(t *testing.T) {
		found := mustLint(t, depguardRoot, rule)
		got := strings.Join(found, "\n")
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("no finding mentions %q; found:\n%s", w, got)
			}
		}
		for _, f := range found {
			if file, _, _ := strings.Cut(f, ":"); strings.HasSuffix(file, "_test.go") {
				t.Errorf("a test file is reported: %s", f)
			}
		}
	})
}

// mustLint returns the findings of the lists of rule on the module at, running depguard on it once per test binary.
func mustLint(t *testing.T, at, rule string) []string {
	t.Helper()
	v, _ := linted.LoadOrStore(at, &lintRun{})
	run := v.(*lintRun)
	run.once.Do(func() { run.found, run.err = depguard(at) })
	if run.err != nil {
		t.Fatal(run.err)
	}
	var out []string
	for _, f := range run.found {
		if ofRule(f, rule) {
			out = append(out, f)
		}
	}
	return out
}

// ofRule reports whether the finding f comes from a list of rule: golangci-lint prints the name of the list in
// lower case, between quotes.
func ofRule(f, rule string) bool {
	_, list, ok := strings.Cut(f, "from list '")
	if !ok {
		return false
	}
	list, _, _ = strings.Cut(list, "'")
	r := strings.ToLower(rule)
	return list == r || strings.HasPrefix(list, r+"-")
}

// depguard runs golangci-lint with only depguard and the .golangci.yml of the module under check on every package
// of the module at, and returns its findings. It fails when golangci-lint is missing, ends with an error other than its
// findings, or reports a finding of another linter (a package that does not compile), with its output.
func depguard(at string) ([]string, error) {
	bin, err := exec.LookPath("golangci-lint")
	if err != nil {
		return nil, fmt.Errorf("the rules that depguard decides need golangci-lint: %s: %w", installLint, err)
	}
	dir, err := realPath(at)
	if err != nil {
		return nil, err
	}
	config, err := filepath.Abs(filepath.Join(*root, ".golangci.yml"))
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, "run", "--enable-only", "depguard", "--config", config, "--allow-parallel-runners",
		"--path-mode", "abs", "--show-stats=false", "--output.json.path", "stdout", "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) { // 1: findings
		return nil, fmt.Errorf("golangci-lint on %s: %w\n%s%s", at, err, stdout.String(), stderr.String())
	}
	var report struct{ Issues []lintIssue }
	if err := json.NewDecoder(bytes.NewReader(stdout.Bytes())).Decode(&report); err != nil {
		return nil, fmt.Errorf("golangci-lint on %s: its output is not JSON: %w\n%s%s", at, err, stdout.String(),
			stderr.String())
	}
	var out []string
	for _, is := range report.Issues {
		if is.FromLinter != "depguard" {
			return nil, fmt.Errorf("golangci-lint on %s reports %s, not depguard: %s:%d: %s", at, is.FromLinter,
				is.Pos.Filename, is.Pos.Line, is.Text)
		}
		file, err := realPath(is.Pos.Filename)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(dir, file)
		if err != nil {
			return nil, err
		}
		f := fmt.Sprintf("%s:%d: %s", filepath.ToSlash(rel), is.Pos.Line, is.Text)
		out = append(out, f)
	}
	return out, nil
}

// realPath returns the absolute path of p with its links resolved, as golangci-lint may name the files it reports.
func realPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// readDepguardList reads the list name of depguard from the .golangci.yml of the module under check, with the
// module path its entries name.
func readDepguardList(name string) (depguardList, string, error) {
	data, err := os.ReadFile(filepath.Join(*root, ".golangci.yml"))
	if err != nil {
		return depguardList{}, "", err
	}
	var doc struct {
		Linters struct {
			Settings struct {
				Depguard struct {
					Rules map[string]depguardList `yaml:"rules"`
				} `yaml:"depguard"`
			} `yaml:"settings"`
		} `yaml:"linters"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return depguardList{}, "", fmt.Errorf(".golangci.yml: %w", err)
	}
	list, ok := doc.Linters.Settings.Depguard.Rules[name]
	if !ok {
		return depguardList{}, "", fmt.Errorf(".golangci.yml has no list %s of depguard", name)
	}
	return list, modulePath(*root), nil
}

// allows reports whether the list lets a file import p, as depguard decides: denied when an entry of deny matches
// it, else allowed when allow is empty or an entry of allow matches it.
func (l depguardList) allows(p string) bool {
	for _, d := range l.Deny {
		if depguardMatch(d.Pkg, p) {
			return false
		}
	}
	if len(l.Allow) == 0 {
		return true
	}
	for _, a := range l.Allow {
		if depguardMatch(a, p) {
			return true
		}
	}
	return false
}

// depguardMatch reports whether the entry e of a list of depguard matches the import p: $gostd any package of the
// standard library (its first element has no dot), an entry ending in $ that package alone, any other entry the
// package and those below it, as a prefix.
func depguardMatch(e, p string) bool {
	switch {
	case e == "$gostd":
		first, _, _ := strings.Cut(p, "/")
		return !strings.Contains(first, ".")
	case strings.HasSuffix(e, "$"):
		return p == strings.TrimSuffix(e, "$")
	}
	return strings.HasPrefix(p, e)
}

// TestDepguardWithoutGolangciLint checks that the run of depguard fails, saying how to install golangci-lint, when
// it is not on the PATH, so the tests of the rules that depguard decides fail instead of skipping.
func TestDepguardWithoutGolangciLint(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := depguard(depguardRoot)
	if err == nil || !strings.Contains(err.Error(), "install golangci-lint") {
		t.Fatalf("depguard without golangci-lint: got error %v, want one that says how to install it", err)
	}
}

// TestDepguardMatch checks that the lists of .golangci.yml read as depguard reads them: deny wins over allow, a
// list with allow denies what it does not allow, and $ ends an exact entry.
func TestDepguardMatch(t *testing.T) {
	l := depguardList{Allow: []string{"$gostd", "example.com/app/internal/app", "example.com/app/lib$"},
		Deny: []depguardDeny{{Pkg: "os$"}}}
	for p, want := range map[string]bool{
		"strings": true, "os": false, "os/exec": true, "example.com/app/internal/app/rule/check": true,
		"example.com/app/lib": true, "example.com/app/lib/sub": false, "example.com/other": false,
	} {
		if got := l.allows(p); got != want {
			t.Errorf("allows(%q) = %v, want %v", p, got, want)
		}
	}
}
