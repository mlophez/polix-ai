// Package rules loads the rules of ai/rules/*.yaml and runs the check of the deterministic ones.
//
// Polix only reads the fields it needs (id, description, scope, deterministic, check); the rest of the
// rule format belongs to the skills that write and use the rules.
package rules

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/internal/glob"
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
	files, err := filepath.Glob(filepath.Join(root, Dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	var all []Rule
	for _, f := range files {
		if filepath.Base(f) == retiredFile {
			continue
		}
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
	var cmd *exec.Cmd
	switch {
	case runtime.GOOS == "windows" && strings.HasSuffix(check, ".py"):
		cmd = exec.CommandContext(ctx, "python", script)
	case runtime.GOOS == "windows" && strings.HasSuffix(check, ".sh"):
		cmd = exec.CommandContext(ctx, "bash", script)
	default:
		cmd = exec.CommandContext(ctx, script)
	}
	cmd.Dir = root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// tail keeps the last n non-empty lines of s.
func tail(s string, n int) string {
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
