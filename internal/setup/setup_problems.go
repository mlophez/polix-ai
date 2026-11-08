package setup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/version"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// WorkflowProblems returns the problems of loading polix.yaml, from the error of loading it: one per problem of a
// workflow that parses (workflow.Invalid), else the error itself; and whether they may come from a newer polix
// than this one, which never holds when the workflow requires one (workflow.TooOld says so).
func WorkflowProblems(err error) ([]string, bool) {
	if err == nil {
		return nil, false
	}
	problems := []string{err.Error()}
	if invalid, ok := errors.AsType[workflow.Invalid](err); ok {
		problems = invalid
	}
	_, old := errors.AsType[workflow.TooOld](err)
	return problems, !old && version.NeedsNewer(strings.Join(problems, "\n"))
}

// RuleProblems returns the problems of the rules, every one of them: those of each invalid rule (invalid, from
// rule.ParseAll), the ids repeated, and the checks that do not exist (missing, by check).
func RuleProblems(rules []rule.Rule, invalid []string, missing map[string]bool) []string {
	problems := append([]string{}, invalid...)
	problems = append(problems, rule.Repeated(rules)...)
	for _, r := range rules {
		if r.Valid() == nil && r.HasCheck() && missing[r.Check] {
			problems = append(problems, fmt.Sprintf("%s %s: check %s does not exist", r.File, r.ID, r.Check))
		}
	}
	return problems
}

// RunProblems returns the checks that cannot run as programs (runnable, by check: not executable, or on Windows
// without an interpreter), each problem once. A check that a runner of rules.run in wf runs is never one of them:
// its runner runs it.
func RunProblems(rules []rule.Rule, wf workflow.Workflow, runnable map[string]error) []string {
	var problems []string
	seen := map[string]bool{}
	for _, r := range rules {
		err := runnable[r.Check]
		if !r.HasCheck() || err == nil || wf.CheckCommand(r.Check, r.ID) != nil || seen[err.Error()] {
			continue
		}
		seen[err.Error()] = true
		problems = append(problems, fmt.Sprintf("%s %s: %v", r.File, r.ID, err))
	}
	return problems
}

// BaseWarning explains that polix rules check --changed would fail because no base branch exists (baseErr), when
// a deterministic rule has a check; "" otherwise.
func BaseWarning(rules []rule.Rule, baseErr error) string {
	for _, r := range rules {
		if r.HasCheck() && baseErr != nil {
			return fmt.Sprintf("polix rules check --changed would fail: %v; set base_branch in %s or pass --base", baseErr,
				workflow.File)
		}
	}
	return ""
}

// NewerHint is the line polix config check and polix doctor add to problems that may come from a newer polix than the
// one of version running.
func NewerHint(running string) string {
	return fmt.Sprintf("these problems may be options of a newer polix than this one (%s): run polix update before "+
		"removing them", running)
}

// ParseWorkflow reads polix.yaml (data, which existed or not) for the polix of version running: the workflow,
// whether the file exists, and the error of workflow.Parse, a workflow.Invalid with the workflow when it parses.
func ParseWorkflow(data []byte, existed bool, running string) (workflow.Workflow, bool, error) {
	if !existed {
		data = nil
	}
	wf, err := workflow.Parse(data, running)
	return wf, existed, err
}
