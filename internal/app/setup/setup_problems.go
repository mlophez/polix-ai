package setup

import (
	"errors"
	"fmt"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// WorkflowProblems returns the problems of loading polix.yaml, from the error of loading it (WorkflowErr): one
// per problem of a workflow that parses (workflow.Invalid), else the error itself; and whether they may come from
// a newer polix than this one, which never holds when the workflow requires one (workflow.TooOld says so).
func (i Inspection) WorkflowProblems() ([]string, bool) {
	if i.WorkflowErr == nil {
		return nil, false
	}
	problems := []string{i.WorkflowErr.Error()}
	if invalid, ok := errors.AsType[workflow.Invalid](i.WorkflowErr); ok {
		problems = invalid
	}
	_, old := errors.AsType[workflow.TooOld](i.WorkflowErr)
	return problems, !old && version.Problems(strings.Join(problems, "\n")).NeedsNewer()
}

// RuleProblems returns the problems of the rules, every one of them: those of each invalid rule (Invalid, from
// rule.Loaded.Problems), the ids repeated, and the checks that do not exist (Missing, by check).
func (i Inspection) RuleProblems() []string {
	problems := append([]string{}, i.Invalid...)
	problems = append(problems, rule.Rules(i.Rules).Repeated()...)
	for _, r := range i.Rules {
		if r.Valid() == nil && r.HasCheck() && i.Missing[r.Check] {
			problems = append(problems, fmt.Sprintf("%s %s: check %s does not exist", r.File, r.ID, r.Check))
		}
	}
	return problems
}

// BaseWarning explains that polix rules check --changed would fail because no base branch exists (BaseErr), when
// a deterministic rule has a check; "" otherwise.
func (i Inspection) BaseWarning() string {
	for _, r := range i.Rules {
		if r.HasCheck() && i.BaseErr != nil {
			return fmt.Sprintf("polix rules check --changed would fail: %v; set base_branch in %s or pass --base",
				i.BaseErr, workflow.File)
		}
	}
	return ""
}

// NewerHint is the line polix config check and polix doctor add to problems that may come from a newer polix than
// the one of the version of the setup, the running one.
func (i Inspection) NewerHint() string {
	return fmt.Sprintf("these problems may be options of a newer polix than this one (%s): run polix update before "+
		"removing them", i.Setup.Version)
}
