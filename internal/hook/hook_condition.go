package hook

import (
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/glob"
	"github.com/mlophez/polix-ai/pkg/shell"
)

// Vars returns the variables of the workflow for the branch; one whose expression does not match is "".
func Vars(wf workflow.Workflow, branch string) map[string]string {
	vars := map[string]string{}
	for name, v := range wf.Vars {
		re, err := regexp.Compile(v.Branch)
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatch(branch); m != nil {
			vars[name] = m[0]
			if len(m) > 1 {
				vars[name] = m[1]
			}
		}
	}
	return vars
}

// Expand replaces {path}, {command}, {branch} and the variables in s.
func Expand(s string, ev agent.Event, branch string, vars map[string]string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	repl := []string{"{path}", ev.Path, "{command}", ev.Command, "{branch}", branch}
	for k, v := range vars {
		repl = append(repl, "{"+k+"}", v)
	}
	return strings.NewReplacer(repl...).Replace(s)
}

// MatchingCommands returns the commands of the line that match one of the patterns.
func MatchingCommands(patterns []string, command string) []shell.Segment {
	var out []shell.Segment
	for _, seg := range shell.Split(command) {
		for _, p := range patterns {
			if glob.Command(p, seg.Text) {
				out = append(out, seg)
				break
			}
		}
	}
	return out
}

// Propagates reports whether the failure of one of the commands reaches the exit code of the line: in
// `just test | tail`, tail decides it, so a fact like tested must not be recorded from it.
func Propagates(segs []shell.Segment) bool {
	for _, seg := range segs {
		if seg.Propagates {
			return true
		}
	}
	return false
}

// Branch reports whether the branch matches the regular expression of a condition branch; an expression that
// does not compile never holds.
func Branch(expr, branch string) bool {
	re, err := regexp.Compile(expr)
	return err == nil && re.MatchString(branch)
}

// Indent indents every line of s by two spaces, for the outputs inside a reason.
func Indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}

// OnlyPolix reports whether a command line only checks the workflow or updates polix (`polix check`, `polix
// update`, `polix doctor`, `polix version`, maybe after a cd): what an invalid or too new workflow needs.
func OnlyPolix(command string) bool {
	segs := shell.Split(command)
	for _, seg := range segs {
		if !slices.ContainsFunc(onlyPolixCommands, func(p string) bool { return glob.Command(p, seg.Text) }) {
			return false
		}
	}
	return len(segs) > 0
}

var onlyPolixCommands = []string{"cd *", "polix check*", "polix update*", "polix doctor*", "polix version*"}
