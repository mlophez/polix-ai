package rule

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// Run is one invocation ready to run: the rules it decides, in id order, and the argv its runner of rules.run in
// polix.yaml gives, with the placeholders expanded. More than one rule means a shared invocation, where the plural
// placeholders cover them all. An empty argv runs the check of its only rule as a program, as when no runner
// matches. It is built with a literal followed by Valid.
type Run struct {
	Rules []Rule
	Argv  []string // the command that runs the checks, from the root; empty runs the file as a program
}

// Valid reports why the run is not the run of checks: it needs at least one rule, each with a check, and an argv
// when it has several rules, since only a single check runs as a program. A rule that is not Valid but has a check
// still runs, as the hook always ran it; polix rules check stops on such a rule before it plans.
func (r Run) Valid() error {
	if len(r.Rules) == 0 {
		return errors.New("a run decides at least one rule")
	}
	for _, rl := range r.Rules {
		if !rl.HasCheck() {
			return fmt.Errorf("rule %s has no check to run", rl.ID)
		}
	}
	if len(r.Rules) > 1 && len(r.Argv) == 0 {
		return errors.New("a run of several rules needs a command")
	}
	return nil
}

// Plan turns the rules into the runs that decide them through the runners of wf (Workflow.CheckCommands): the
// rules with a check, in order, grouped as their runners group them. Rules without a check have nothing to run.
func Plan(rules []Rule, wf workflow.Workflow) []Run {
	var with []Rule
	var files, ids []string
	for _, rl := range rules {
		if rl.HasCheck() {
			with = append(with, rl)
			files, ids = append(files, rl.Check), append(ids, rl.ID)
		}
	}
	var out []Run
	for _, inv := range wf.CheckCommands(files, ids) {
		var decided []Rule
		for _, p := range inv.Checks {
			decided = append(decided, with[p])
		}
		run := Run{Rules: decided, Argv: inv.Argv}
		if err := run.Valid(); err != nil {
			continue // never happens: the rules were filtered and the invocations are valid
		}
		out = append(out, run)
	}
	return out
}

// Settle decides the rules of the run from the outcome of carrying it out: when it passed, every rule passes with
// the shared output; when it failed with one rule, that rule fails. When a shared run failed it returns, instead
// of results, a run of each rule alone with the argv of wf for that rule, which decide them; when that argv is the
// shared one for every rule, rerunning would repeat it, so every rule fails with the shared output.
func (r Run) Settle(o Outcome, wf workflow.Workflow) ([]Result, []Run) {
	var reruns []Run
	split := false
	for _, rl := range r.Rules {
		run := Run{Rules: []Rule{rl}, Argv: wf.CheckCommand(rl.Check, rl.ID)}
		if err := run.Valid(); err != nil {
			continue // never happens: the rules of a valid run have a check
		}
		split = split || !slices.Equal(run.Argv, r.Argv)
		reruns = append(reruns, run)
	}
	if o.Err != nil && len(r.Rules) > 1 && split {
		return nil, reruns
	}
	var results []Result
	for _, rl := range r.Rules {
		res := Result{Rule: rl, Output: o.Output, Err: o.Err}
		if err := res.Valid(); err != nil && !rl.HasCheck() {
			continue // never happens, as above; a rule that is not Valid but has a check is still decided
		}
		results = append(results, res)
	}
	return results, nil
}
