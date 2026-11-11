package answer

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/githook"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/glob"
)

// Output is the decision of the logic of AnswerGithook for one call of git: the next run the shell carries out or,
// once there is none, the answer: whether the commit is aborted and why.
type Output struct {
	Run     []string // the command to run next from the root of the repository; nil when the answer is ready
	Block   bool     // the commit is aborted
	Reasons []string // why it is aborted, in order, each a text for the terminal of the commit
}

// evaluation is the evaluation of the entries of one call: the call, the variables of vars its branch gives and,
// on commit_msg, the first line of its message.
type evaluation struct {
	g       githook.Githook
	vars    map[string]string // the value of each variable of vars whose expression matches the branch
	subject string            // the first line of the message (see subject); "" on pre_commit
}

// scissors is the line, after the comment string, from which git drops the rest of a message (git commit --verbose
// writes the diff below it).
const scissors = " ------------------------ >8 ------------------------"

// Decide decides the next step of a call of git g with the workflow wf of polix.yaml: without polix.yaml (present)
// the commit goes on; a workflow that cannot be used (invalid, its problems or a newer polix it requires) aborts it
// with its problems. Else it walks the entries of the githooks event of the call in file order: an entry applies when
// every condition of its when holds and, with an unless, not every condition of its unless does. A block aborts the
// commit with its message; a run asks for its command, once per run carried out so far (githook.Githook.Outcomes),
// and when it failed aborts the commit with its message and the line `<command> failed: <error>`. The first entry
// that aborts ends the evaluation; with none, the commit goes on.
func Decide(g githook.Githook, wf workflow.Workflow, present bool, invalid error) Output {
	switch {
	case !present:
		return Output{}
	case invalid != nil:
		return Output{Block: true, Reasons: problems(invalid)}
	}
	e := evaluation{g: g, vars: branchVars(wf, g.Branch), subject: subject(g.Message, g.Comment)}
	ran := 0
	for _, entry := range wf.Githooks[g.Event] {
		if !e.applies(entry) {
			continue
		}
		if b := entry.Action.Block; b != nil {
			return Output{Block: true, Reasons: []string{e.expand(b.Message)}}
		}
		r := entry.Action.Run
		if r == nil {
			continue
		}
		argv := make([]string, len(r.Command))
		for i, a := range r.Command {
			argv[i] = e.expand(a)
		}
		if ran == len(g.Outcomes) {
			return Output{Run: argv}
		}
		if o := g.Outcomes[ran]; !o.Passed() {
			return Output{Block: true, Reasons: e.failed(r.Message, argv, o)}
		}
		ran++
	}
	return Output{}
}

// problems returns the problems of a workflow that cannot be used, one per reason: each problem of an
// InvalidError, or the error whole.
func problems(err error) []string {
	if invalid, ok := errors.AsType[workflow.InvalidError](err); ok {
		return slices.Clone([]string(invalid))
	}
	return []string{err.Error()}
}

// applies reports whether the entry applies to the call: its when holds and, with an unless, its unless does not.
func (e evaluation) applies(entry workflow.Entry) bool {
	return e.holds(entry.When) && (len(entry.Unless) == 0 || !e.holds(entry.Unless))
}

// holds reports whether every condition of c holds for the call, in the order of their names; an empty c holds.
func (e evaluation) holds(c workflow.Cond) bool {
	for _, k := range slices.Sorted(maps.Keys(c)) {
		if !e.condition(k, c) {
			return false
		}
	}
	return true
}

// condition evaluates the condition k of c: branch, a regular expression the branch matches; changed, globs of which
// one matches a staged file; message, regular expressions of which one matches the first line of the message, with
// the values of their placeholders matched literally. Any other condition, which the workflow does not allow in
// githooks, does not hold, nor does a pattern that does not compile once its placeholders are expanded.
func (e evaluation) condition(k string, c workflow.Cond) bool {
	switch k {
	case "branch":
		s, _ := c[k].(string)
		re, err := regexp.Compile(s)
		return err == nil && re.MatchString(e.g.Branch)
	case "changed":
		patterns := e.list(c, k, e.expand)
		for _, f := range e.g.Staged {
			if glob.AnyPath(patterns, f) {
				return true
			}
		}
	case "message":
		for _, p := range e.list(c, k, e.pattern) {
			if re, err := regexp.Compile(p); err == nil && re.MatchString(e.subject) {
				return true
			}
		}
	}
	return false
}

// list returns the strings of the condition k of c with their placeholders expanded by expand.
func (e evaluation) list(c workflow.Cond, k string, expand func(string) string) []string {
	s, _ := c.Strings(k)
	out := make([]string, len(s))
	for i := range s {
		out[i] = expand(s[i])
	}
	return out
}

// failed returns the reasons of a run of argv that failed with the outcome o: its message, expanded, when it has
// one, and the line `<command> failed: <error>`.
func (e evaluation) failed(message string, argv []string, o githook.Outcome) []string {
	var reasons []string
	if message != "" {
		reasons = append(reasons, e.expand(message))
	}
	return append(reasons, fmt.Sprintf("%s failed: %v", strings.Join(argv, " "), o.Err))
}

// expand replaces {branch}, {message} on commit_msg and the variables of vars in s with their values as they are; a
// variable whose expression does not match the branch stays as written.
func (e evaluation) expand(s string) string {
	return e.replace(s, func(v string) string { return v })
}

// pattern replaces the placeholders of the regular expression s as expand does, with their values quoted, so a
// value is matched literally: its metacharacters neither break nor widen the pattern.
func (e evaluation) pattern(s string) string {
	return e.replace(s, regexp.QuoteMeta)
}

// replace replaces {branch}, {message} on commit_msg and the variables of vars in s with their values passed through
// value.
func (e evaluation) replace(s string, value func(string) string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	repl := []string{"{branch}", value(e.g.Branch)}
	if e.g.Event == workflow.CommitMsg {
		repl = append(repl, "{message}", value(e.subject))
	}
	for _, k := range slices.Sorted(maps.Keys(e.vars)) {
		repl = append(repl, "{"+k+"}", value(e.vars[k]))
	}
	return strings.NewReplacer(repl...).Replace(s)
}

// subject returns the first line of the message raw as git commits it: the lines that start with the comment string
// (# when empty) and everything from the scissors line on are removed, and so are the blank lines before it and the
// blanks at its end; "" when nothing is left.
func subject(raw, comment string) string {
	if comment == "" {
		comment = "#"
	}
	for line := range strings.SplitSeq(raw, "\n") {
		line = strings.TrimRight(line, " \t\r")
		switch {
		case line == comment+scissors:
			return ""
		case strings.HasPrefix(line, comment), line == "":
			continue
		}
		return line
	}
	return ""
}

// branchVars returns the variables of the workflow for the branch: the first group of the expression of each, or the
// whole match; one whose expression does not match is left out.
func branchVars(wf workflow.Workflow, branch string) map[string]string {
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
