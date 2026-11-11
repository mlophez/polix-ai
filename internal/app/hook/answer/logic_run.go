package answer

import (
	"fmt"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/hook"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/text"
)

// outputLines is how many lines of the output of a failing command its reason keeps, the last ones.
const outputLines = 15

// runEntry runs the verb run of the entry e and reports whether the event is actually blocked, which ends its
// evaluation. Without always, a run whose key already passed on the current branch or on the same HEAD commit, and
// on the content hash, is skipped, whoever's entry ran it. A run that passes on a content that did not change during it writes an observed `passed`
// line, with or without always; one during which the content changed, or with an unknown hash, writes none, and the
// hash after the run is only computed when the one before it is known. A
// failure, with the message of the run before its output, blocks the event as a block does (see block), so the
// loop guard may let it through; on the events that already happened (workflow.Informed) it reaches the AI as
// context instead, as feedback, and never blocks. Any run may rewrite files, so the content hash and the files read
// after it are those of a new epoch.
func (r *pass) runEntry(e workflow.Entry, ans agent.Answer) (agent.Answer, bool) {
	run := *e.Action.Run
	args, key := r.runKey(run)
	hash, known := r.hash()
	if r.waiting() {
		return ans, false
	}
	if !run.Always && known {
		passed := r.passed(key, hash)
		if r.waiting() {
			return ans, false
		}
		if passed {
			r.log(workflow.VerbRun, e.Line, "skip", key+" already passed on this content")
			return ans, false
		}
	}
	message := r.expand(run.Message)
	if r.waiting() {
		return ans, false
	}
	reason := r.run(run, args)
	if r.waiting() {
		return ans, false
	}
	r.epoch++
	if reason == "" {
		if !known {
			return ans, false // the content before the run is unknown: the one after it would prove nothing
		}
		if after, knownAfter := r.hash(); knownAfter && after == hash {
			_ = r.write(state.KindObserved, "", e.Line, state.OutcomePassed, "", key, hash)
		}
		return ans, false
	}
	if message != "" {
		reason = message + "\n\n" + reason
	}
	if r.informed {
		r.log(workflow.VerbRun, e.Line, "inform", reason)
		return ans.Inform(reason), false
	}
	return r.block(e, ans, reason, key)
}

// runKey returns the arguments of the command of run, expanded, and the key of its repeat: those arguments joined
// by single spaces, or for the checks of the rules the word rules and the expanded target. A {path} in the command
// or the target makes the key differ per path.
func (r *pass) runKey(run workflow.Run) ([]string, string) {
	if run.Rules != "" {
		return nil, "rules " + r.expand(run.Rules)
	}
	args := make([]string, len(run.Command))
	for i, a := range run.Command {
		args[i] = r.expand(a)
	}
	return args, strings.Join(args, " ")
}

// passed reports whether a run of key passed on the current branch or on the same HEAD commit, and on the content
// hash. A log that cannot be read never counts as passed: the run runs again.
func (r *pass) passed(key, hash string) bool {
	branch, head := r.branch(), r.head()
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomePassed, Branch: branch, Head: head, ByBranch: true, Key: key, Hash: hash,
		ByHash: true}
	if q.Valid() != nil {
		return false
	}
	return st.Index(q) >= 0
}

// run executes the command of run, with its expanded args, or the checks of the rules and returns why it failed,
// or "".
func (r *pass) run(run workflow.Run, args []string) string {
	if run.Rules != "" {
		return r.runRules(run.Rules)
	}
	s := hook.Step{Run: args}
	o := r.act(s, s.Valid())
	if r.waiting() || o.Err == nil {
		return ""
	}
	return fmt.Sprintf("%s failed: %v%s\n%s", strings.Join(args, " "), o.Err, cut(o.Output),
		indent(text.LastLines(o.Output, outputLines, rule.MaxLineLen)))
}

// cut tells how much of output a reason keeps when it keeps only its last outputLines lines, as
// " (last 15 of 412 lines)", so the AI knows it sees a part and can run the command to read the rest; "" when the
// whole output fits.
func cut(output string) string {
	n := strings.Count(strings.TrimRight(output, "\n"), "\n") + 1
	if n <= outputLines {
		return ""
	}
	return fmt.Sprintf(" (last %d of %d lines)", outputLines, n)
}

// runRules runs the checks of the rules that apply to the changed files (`changed`) or to a path, through the
// runners of rules.run that match their files or as programs, grouped as the runners group them (Rules.Plan); a
// failed shared run is rerun per rule (Run.Settle). It returns the rules that failed, or "".
func (r *pass) runRules(target string) string {
	all, err := r.rules()
	if r.waiting() {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("The rules cannot be loaded: %v", err)
	}
	var paths []string
	if target == "changed" {
		paths = r.changed()
	} else {
		paths = []string{r.expand(target)}
	}
	var results []rule.Result
	queue := rule.Rules(all).Matching(paths).Plan(r.ev.Workflow)
	for len(queue) > 0 {
		run := queue[0]
		s := hook.Step{Check: &run}
		o := r.act(s, s.Valid())
		if r.waiting() {
			return ""
		}
		outcome := rule.Outcome{Output: o.Output, Err: o.Err}
		if err := outcome.Valid(); err != nil {
			return ""
		}
		res, reruns := run.Settle(outcome, r.ev.Workflow)
		results = append(results, res...)
		queue = append(reruns, queue[1:]...)
	}
	failed := rule.Results(results).Failed()
	if len(failed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Rules not met:")
	for _, f := range failed {
		fmt.Fprintf(&b, "\n- %s (%s): %s\n%s", f.Rule.ID, f.Rule.Check, f.Rule.Description, indent(f.Tail()))
	}
	return b.String()
}

// indent indents every line of s by two spaces, for the outputs inside a reason.
func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}
