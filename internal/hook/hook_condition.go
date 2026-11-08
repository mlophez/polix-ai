package hook

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/diff"
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

// Expand replaces {path}, {command}, {branch}, the variables and the measures in s. The measures are the values of
// {changed_lines}, {changed_files} and {risk_score} (see Measures); nil leaves them as they are.
func Expand(s string, ev agent.Event, branch string, vars, measures map[string]string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	repl := []string{"{path}", ev.Path, "{command}", ev.Command, "{branch}", branch}
	for k, v := range measures {
		repl = append(repl, "{"+k+"}", v)
	}
	for k, v := range vars {
		repl = append(repl, "{"+k+"}", v)
	}
	return strings.NewReplacer(repl...).Replace(s)
}

// Measures returns the values of the measure placeholders of d, with the weights of the risk score: integers,
// or "?" for each when the diff could not be measured.
func Measures(d diff.Diff, weights []workflow.Weight, measured bool) map[string]string {
	if !measured {
		return map[string]string{"changed_lines": "?", "changed_files": "?", "risk_score": "?"}
	}
	return map[string]string{
		"changed_lines": strconv.Itoa(d.Lines()),
		"changed_files": strconv.Itoa(d.Count()),
		"risk_score":    strconv.Itoa(d.Risk(weights)),
	}
}

// UsesMeasures reports whether s holds a measure placeholder, so the diff is read only when a string needs it.
func UsesMeasures(s string) bool {
	return strings.Contains(s, "{changed_lines}") || strings.Contains(s, "{changed_files}") ||
		strings.Contains(s, "{risk_score}")
}

// Measure reports whether the measure condition k holds on d: its lines, its files or its risk score with the
// weights, at least m.AtLeast.
func Measure(k string, m workflow.Measure, d diff.Diff, weights []workflow.Weight) bool {
	switch k {
	case "changed_lines":
		return d.Lines() >= m.AtLeast
	case "changed_files":
		return d.Count() >= m.AtLeast
	case "risk_score":
		return d.Risk(weights) >= m.AtLeast
	}
	return false
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
// `just test | tail`, tail decides it, so the exit code of the line says nothing of how just test ended.
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

// OnlyPolix reports whether a command line only checks the workflow or updates polix (`polix config check`, `polix
// config fmt --check`, `polix update`, `polix doctor`, `polix version`, maybe after a cd): what an invalid or too new
// workflow needs. `polix config fmt` writes polix.yaml, so only its exact form with --check is one of them: any
// other flag after it (--check=false) could turn the write back on.
func OnlyPolix(command string) bool {
	segs := shell.Split(command)
	for _, seg := range segs {
		if !slices.ContainsFunc(onlyPolixCommands, func(p string) bool { return glob.Command(p, seg.Text) }) {
			return false
		}
	}
	return len(segs) > 0
}

var onlyPolixCommands = []string{"cd *", "polix config check*", "polix config fmt --check", "polix update*",
	"polix doctor*", "polix version*"}
