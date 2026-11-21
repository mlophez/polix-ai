package answer

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/diff"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// expand replaces {var}, {path}, {command}, {branch}, {commit}, {subject} and the measures in s. The diff is read
// only when s holds a measure placeholder, and measured over the globs of the `changed` of the `when` of the entry,
// else whole. {commit} and {subject} are the short hash and the subject of the commit just made on post_commit, read
// only when s holds one of them, and an empty text on any other event or when the commit cannot be read.
func (r *pass) expand(s string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	branch := r.branch()
	vars := branchVars(r.ev.Workflow, branch)
	var measures map[string]string
	if usesMeasures(s) {
		d, measured := r.diff()
		scope := r.whenScope()
		if r.waiting() {
			return s
		}
		measures = measureValues(d.Narrow(scope), r.ev.Workflow.Weights, measured)
	}
	var commit hook.Commit
	if r.onCommit() && usesCommit(s) {
		commit, _ = r.commit()
		if r.waiting() {
			return s
		}
	}
	return expandPlaceholders(s, r.ev.Event, branch, vars, measures, commit)
}

// whenScope returns the globs of the `changed` of the `when` of the entry, expanded without the measures, so a glob
// never asks for the diff it scopes; nil without one.
func (r *pass) whenScope() []string {
	globs, _ := r.entry.When.Strings("changed")
	if len(globs) == 0 {
		return nil
	}
	branch := r.branch()
	vars := branchVars(r.ev.Workflow, branch)
	scope := make([]string, len(globs))
	for i, g := range globs {
		scope[i] = expandPlaceholders(g, r.ev.Event, branch, vars, nil, hook.Commit{})
	}
	return scope
}

// branchVars returns the variables of the workflow for the branch; one whose expression does not match is "".
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

// expandPlaceholders replaces {path}, {command}, {branch}, {commit}, {subject}, the variables and the measures in s.
// The measures are the values of {changed_lines}, {changed_files} and {risk_score} (see measureValues); nil leaves
// them as they are. {commit} and {subject} take the hash and the subject of commit, empty outside post_commit.
func expandPlaceholders(s string, ev agent.Event, branch string, vars, measures map[string]string,
	commit hook.Commit) string {
	if !strings.Contains(s, "{") {
		return s
	}
	repl := []string{"{path}", ev.Path, "{command}", ev.Command, "{branch}", branch, "{commit}", commit.Hash,
		"{subject}", commit.Subject}
	for k, v := range measures {
		repl = append(repl, "{"+k+"}", v)
	}
	for k, v := range vars {
		repl = append(repl, "{"+k+"}", v)
	}
	return strings.NewReplacer(repl...).Replace(s)
}

// measureValues returns the values of the measure placeholders of d, with the weights of the risk score: integers,
// or "?" for each when the diff could not be measured.
func measureValues(d diff.Diff, weights []workflow.Weight, measured bool) map[string]string {
	if !measured {
		return map[string]string{"changed_lines": "?", "changed_files": "?", "risk_score": "?"}
	}
	return map[string]string{
		"changed_lines": strconv.Itoa(d.Lines()),
		"changed_files": strconv.Itoa(d.Count()),
		"risk_score":    strconv.Itoa(d.Risk(weights)),
	}
}

// usesCommit reports whether s holds {commit} or {subject}, so the commit just made is read only when a string needs
// it.
func usesCommit(s string) bool {
	return strings.Contains(s, "{commit}") || strings.Contains(s, "{subject}")
}

// usesMeasures reports whether s holds a measure placeholder, so the diff is read only when a string needs it.
func usesMeasures(s string) bool {
	return strings.Contains(s, "{changed_lines}") || strings.Contains(s, "{changed_files}") ||
		strings.Contains(s, "{risk_score}")
}
