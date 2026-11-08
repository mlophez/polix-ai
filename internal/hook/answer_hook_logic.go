package hook

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/internal/agent"
	"github.com/mlophez/polix-ai/internal/diff"
	"github.com/mlophez/polix-ai/internal/rule"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/glob"
	"github.com/mlophez/polix-ai/pkg/text"
)

// AnswerHookOutput is the decision of the logic of AnswerHook for one event: the next step the shell carries out
// or, once it is none, the answer to the event and the facts the next event of the call starts from.
type AnswerHookOutput struct {
	Step   Step
	Answer agent.Answer
	Facts  Facts
}

// answerHookRun is one pass of the evaluation of an event, from its start: the facts are read from the
// evaluation, and the steps that act take their outcomes from its journal, in order, until one is missing.
type answerHookRun struct {
	ev    Evaluation
	epoch int         // runs carried out so far, which the content hash and the files read depend on
	state state.State // the log as this pass left it
	next  int         // the next outcome of the journal
	step  *Step       // the step the pass waits for; nil while it goes on
	// entry is the entry being evaluated: the `changed` of its `when` scopes the measure placeholders and the
	// content `finished` compares.
	entry workflow.Entry
	// guarded tells that the loop guard let a block of the event through: the event did not end as asked.
	guarded bool
	// warnings are the warnings of the pass for the user, such as a log that cannot be read.
	warnings []string
	// injected are the once-only injects that reached the answer, written as observed `injected` lines once the
	// answer is known (see writeInjected).
	injected []answerHookInjected
}

// answerHookInjected is a once-only inject that reached the answer: the line of its entry and its key.
type answerHookInjected struct {
	line int
	key  string
}

// answerHookOutputLines is how many lines of the output of a failing command its reason keeps, the last ones.
const answerHookOutputLines = 15

// AnswerHook decides the next step of the evaluation of an event: the protections that come before the entries of
// the workflow (nobody edits the state of polix; with a valid workflow, no actor writes a protected file nor runs a
// forbidden command), then, with a valid workflow, its entries in order, each with its conditions and its action. It
// walks the event from its start every time it is asked, so the step it returns is the first one whose result the
// evaluation does not know yet; with none, the event is answered.
func AnswerHook(ev Evaluation) AnswerHookOutput {
	r := &answerHookRun{ev: ev, epoch: ev.Facts.Epoch, state: ev.Facts.State}
	ans := r.handle()
	if r.step != nil {
		return AnswerHookOutput{Step: *r.step}
	}
	return AnswerHookOutput{Answer: ans, Facts: ev.Facts.Carry(r.epoch, r.state)}
}

// handle answers the event: the protections, then the entries of the event in order, until the first one that
// actually blocks, a block or a failing run, whatever its verb (see apply); the entries after it never run, and the
// contexts of those before it reach the agent with the block. With the answer known, it writes the once-only injects
// that reached the AI (see writeInjected). On a subagent_stop the answer does not refuse, it then writes that the
// subagent finished when the workflow observes its type or injects once (see observeFinished), and on start that
// the session started when the workflow injects once, so its once-only injects are due again. It stops early
// when a step is missing; the answer is then meaningless.
func (r *answerHookRun) handle() agent.Answer {
	ans, denied := r.protect()
	if r.waiting() || denied {
		return ans
	}
	if r.ev.Invalid != nil {
		return r.invalid()
	}
	if !r.ev.Present {
		return agent.Answer{}
	}
	if outside := r.outsideChange(); r.waiting() || outside {
		return agent.Answer{}
	}
	// The built-in guidance comes before the entries, so the contexts of the repository have the last word.
	if r.ev.Event.Name == workflow.Start {
		if text := r.ev.Workflow.SessionContext(); text != "" {
			ans = ans.Inform(text)
		}
	}
	for _, e := range r.ev.Workflow.On[r.ev.Event.Name] {
		r.entry = e
		if !r.matches(e.When, nil) || len(e.Unless) > 0 && r.matches(e.Unless, nil) {
			if r.waiting() {
				return ans
			}
			continue
		}
		if r.waiting() {
			return ans
		}
		var stop bool
		ans, stop = r.apply(e, ans)
		if r.waiting() || stop {
			break
		}
	}
	if r.waiting() {
		return ans
	}
	r.writeInjected(ans)
	if r.waiting() {
		return ans
	}
	switch {
	case r.ev.Event.Name == workflow.SubagentStop && !ans.Block:
		r.observeFinished()
	case r.ev.Event.Name == workflow.Start && r.ev.Workflow.InjectsOnce():
		_ = r.write(state.KindObserved, "", 0, state.OutcomeStarted, "", "", "")
	}
	if r.waiting() {
		return ans
	}
	for _, w := range append(slices.Clone(r.warnings), r.ev.Facts.Warnings...) {
		if !slices.Contains(ans.Warnings, w) {
			ans = ans.Warn(w)
		}
	}
	return ans
}

// waiting reports whether the pass waits for a step.
func (r *answerHookRun) waiting() bool { return r.step != nil }

// ask makes s the step the pass waits for, unless it already waits for one or s is not valid.
func (r *answerHookRun) ask(s Step, valid error) {
	if r.step == nil && valid == nil && !s.None() {
		r.step = &s
	}
}

// act returns the outcome of a step that acts, from the journal; when the journal has no more, the pass waits
// for s.
func (r *answerHookRun) act(s Step, valid error) Outcome {
	if r.waiting() {
		return Outcome{}
	}
	if r.next < len(r.ev.Journal) {
		o := r.ev.Journal[r.next]
		r.next++
		return o
	}
	r.ask(s, valid)
	return Outcome{}
}

// protect applies the protections that come before the entries of the workflow and before the skip of the edits
// outside the change, and reports whether one denied the event, logged with where it comes from: nobody edits the
// state of polix, whatever the workflow says, also without one or with an invalid one; and, with a valid workflow,
// no actor, the main session included, writes a path of guardrails.protected_files, also when git ignores it, nor
// runs a command of guardrails.forbidden_commands, wherever it comes in the line. Reading is always allowed.
func (r *answerHookRun) protect() (agent.Answer, bool) {
	ev, fold := r.ev.Event, r.ev.Fold()
	var wf workflow.Workflow
	if r.ev.Invalid == nil && r.ev.Present {
		wf = r.ev.Workflow
	}
	switch ev.Name {
	case workflow.PreEdit:
		abs := Absolute(r.ev.Root, "", ev.AbsPath)
		if abs != "" {
			reals := r.reals(r.ev.StateDir(), abs)
			if r.waiting() {
				return agent.Answer{}, false
			}
			if Within(reals[r.ev.StateDir()], reals[abs], fold) {
				return r.deny(BuiltinProtection, 0, ReasonStateEdit)
			}
		}
		if !ev.Outside && ev.Path != "" && AnyPathFold(wf.ProtectedFiles, ev.Path, fold) {
			return r.deny(ProtectedFilesShortcut, 0, fmt.Sprintf("%s is protected in %s: only a person changes it.",
				ev.Path, workflow.File))
		}
	case workflow.PreCommand, workflow.PreCommit:
		// Reading is fine (the log is the evidence of the process); only commands that could write are denied.
		if MayWrite(ev.Command) {
			reals := r.reals(r.ev.StateDir())
			if r.waiting() {
				return agent.Answer{}, false
			}
			if NamesState(ev.Command, r.ev.StateDir(), reals[r.ev.StateDir()], fold) {
				return r.deny(BuiltinProtection, 0, ReasonStateCommand)
			}
		}
		p, held, ok := r.writes(wf.ProtectedFiles)
		switch {
		case r.waiting():
			return agent.Answer{}, false
		case ok && held != "":
			return r.deny(ProtectedFilesShortcut, 0, fmt.Sprintf("%s holds %s, protected in %s: only a person changes it.",
				p, held, workflow.File))
		case ok:
			return r.deny(ProtectedFilesShortcut, 0, fmt.Sprintf("%s is protected in %s: only a person changes it.", p,
				workflow.File))
		}
		if f, ok := Forbidden(wf.ForbiddenCommands, ev.Command); ok {
			reason := f.Reason
			if reason == "" {
				reason = ReasonForbidden
			}
			return r.deny(ForbiddenCommandsShortcut, f.Line, reason+"\n"+ev.Command)
		}
	}
	return agent.Answer{}, false
}

// deny refuses the event for a protection with reason, logged as a denial of from (BuiltinProtection or the shortcut
// of guardrails) at line, the line of its item or 0.
func (r *answerHookRun) deny(from string, line int, reason string) (agent.Answer, bool) {
	r.log(from, line, "deny", reason)
	return agent.Answer{}.Refuse(reason), true
}

// invalid answers the event when the workflow is not valid. Edits and commands fail closed, so a broken file is
// noticed at once instead of silently disabling the workflow, except what fixing it needs: the main session may
// edit the workflow and run the commands of polix that check or update it, and edits outside the change never
// depended on the workflow. Other events only warn: a blocked stop would keep the agent in a loop it cannot leave.
//
// A workflow that requires a newer polix is not broken but ahead of this binary: the answer asks to update polix,
// and not even the main session may edit the workflow, so the agent does not take options out of it to please an
// old binary.
func (r *answerHookRun) invalid() agent.Answer {
	ev := r.ev.Event
	problems := r.ev.Invalid.Error()
	reason := "polix: el workflow no es válido; corrígelo y ejecuta polix config check.\n" + problems
	var old workflow.TooOld
	tooOld := errors.As(r.ev.Invalid, &old)
	if tooOld {
		problems = fmt.Sprintf("este repositorio necesita polix %s o posterior y la instalada es %s", old.Requires,
			old.Running)
		reason = "polix: " + problems + ". Ejecuta polix update (o pide al usuario que lo haga) y no cambies " +
			workflow.File + "."
	}
	switch ev.Name {
	case workflow.PreEdit:
		outside := r.outsideChange()
		if r.waiting() || outside || (!tooOld && ev.Actor == agent.ActorSession && ev.Path == workflow.File) {
			return agent.Answer{}
		}
	case workflow.PreCommand, workflow.PreCommit:
		if OnlyPolix(ev.Command) {
			return agent.Answer{}
		}
	default:
		if tooOld {
			return agent.Answer{}.Warn("polix: " + problems + ": ejecuta polix update")
		}
		return agent.Answer{}.Warn("el workflow no es válido: " + problems)
	}
	r.log("invalid", 0, "block", problems)
	return agent.Answer{}.Refuse(reason)
}

// outsideChange reports an edit outside the repository (scratchpad, memory, another repository) or of a file
// git ignores: it is not part of the change, so the workflow does not apply to it.
func (r *answerHookRun) outsideChange() bool {
	ev := r.ev.Event
	if !ev.Edits() {
		return false
	}
	return ev.Outside || r.ignored(ev.Path)
}

// apply carries out the verb of the action of a matching entry and reports whether the evaluation of the event
// stops there. One rule, whatever the verb: the first entry that actually blocks ends the evaluation, a `block` or
// a failing `run` on an event where it blocks. A block let through by the loop guard, an inject, a skipped verb, a
// passing run and a failing run on an event where it only informs let it go on.
func (r *answerHookRun) apply(e workflow.Entry, ans agent.Answer) (agent.Answer, bool) {
	a := e.Action
	switch a.Verb() {
	case workflow.VerbBlock:
		if a.Block.Once {
			reminded := r.reminded(e)
			if r.waiting() {
				return ans, false
			}
			if reminded {
				r.log(workflow.VerbBlock, e.Line, "skip", "ya bloqueó a "+r.ev.Event.Actor+
					" en esta sesión sobre el mismo contenido")
				return ans, false
			}
		}
		reason := r.expand(a.Block.Message)
		if r.waiting() {
			return ans, false
		}
		return r.block(e, ans, reason, "")
	case workflow.VerbRun:
		return r.runEntry(e, ans)
	case workflow.VerbInject:
		return r.inject(e, ans), false
	}
	return ans, false
}

// block blocks the event for the entry e and reports whether it is actually blocked; key is what a failing run ran,
// "" for a block. A block with once writes an observed `blocked` line, which reminded reads. On stop and
// subagent_stop every block (a block or a failing run) writes it too, for the loop guard: after max_stop_retries
// blocks of this entry of the actor in the session on this same content (the whole hash), the agent may stop and a
// trace is left. An unknown hash counts as one content (state.UnknownHash). When the log cannot be read or written
// the guard cannot count, so it fails open. A block with once outside the stops blocks without writing when the
// hash is unknown or the log cannot be read, so it blocks again.
func (r *answerHookRun) block(e workflow.Entry, ans agent.Answer, reason, key string) (agent.Answer, bool) {
	ev := r.ev.Event
	verb := e.Action.Verb()
	stop := ev.Name == workflow.Stop || ev.Name == workflow.SubagentStop
	once := e.Action.Block != nil && e.Action.Block.Once
	if !stop && !once {
		r.log(verb, e.Line, "block", reason)
		return ans.Refuse(reason), true
	}
	hash, known := r.hash()
	st, err := r.currentState()
	if r.waiting() {
		return ans, false
	}
	if stop {
		if err != nil {
			return r.release(e, ans, fmt.Sprintf("polix deja parar porque no puede usar su log (%v): %s", err, reason),
				reason), false
		}
		if !known {
			hash, known = state.UnknownHash, true
		}
		q := state.Query{Outcome: state.OutcomeBlocked, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
			Line: e.Line, Hash: hash, ByHash: true}
		if err := q.Valid(); err != nil {
			return r.release(e, ans, fmt.Sprintf("polix deja parar porque no puede usar su log (%v): %s", err, reason),
				reason), false
		}
		if n := len(st.Observed(q)); n >= r.ev.Workflow.MaxStopRetries {
			return r.release(e, ans, fmt.Sprintf("polix deja parar tras %d bloqueos: %s", n, reason), reason), false
		}
	}
	if known && err == nil {
		werr := r.write(state.KindObserved, "", e.Line, state.OutcomeBlocked, "", key, hash)
		if r.waiting() {
			return ans, false
		}
		if werr != nil && stop {
			return r.release(e, ans, fmt.Sprintf("polix deja parar porque no puede usar su log (%v): %s", werr, reason),
				reason), false
		}
	}
	r.log(verb, e.Line, "block", reason)
	return ans.Refuse(reason), true
}

// release lets the stop through for the loop guard, with warning for the user: the block of the entry e is logged
// as loop_guard with its reason, and the event did not end as asked.
func (r *answerHookRun) release(e workflow.Entry, ans agent.Answer, warning, reason string) agent.Answer {
	r.guarded = true
	r.log(e.Action.Verb(), e.Line, "loop_guard", reason)
	return ans.Warn(warning)
}

// reminded reports whether the block with once of the entry e already blocked the actor of the event in its
// session on the current branch, on a content whose files in the scope of the entry did not change since: the tree
// of its latest observed `blocked` line is compared with the current one. An unknown hash, a log that cannot be
// read and two trees that cannot be compared never count as reminded, the last with the error in the log: the
// reminder blocks again.
func (r *answerHookRun) reminded(e workflow.Entry) bool {
	ev := r.ev.Event
	branch := r.branch()
	hash, known := r.hash()
	st, err := r.currentState()
	if r.waiting() || !known {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomeBlocked, Session: ev.Session, Actor: ev.Actor, Line: e.Line, Branch: branch,
		ByBranch: true}
	if q.Valid() != nil {
		return false
	}
	last, ok := st.Latest(q)
	if !ok || last.Hash == "" || last.Hash == state.UnknownHash {
		return false
	}
	same, err := r.sameScope(last.Hash, hash, r.whenScope())
	if err != nil {
		r.log(workflow.VerbBlock, e.Line, "warn", fmt.Sprintf(
			"no se puede comparar el contenido en que se recordó con el actual (%v): el block vuelve a bloquear", err))
		return false
	}
	return same
}

// runEntry runs the verb run of the entry e and reports whether the event is actually blocked, which ends its
// evaluation. Without always, a run whose key already passed on the current branch and content hash is skipped,
// whoever's entry ran it. A run that passes on a content that did not change during it writes an observed `passed`
// line, with or without always; one during which the content changed, or with an unknown hash, writes none, and the
// hash after the run is only computed when the one before it is known. A
// failure, with the message of the run before its output, blocks the event as a block does (see block), so the
// loop guard may let it through; on the events that already happened (workflow.Informed) it reaches the AI as
// context instead, as feedback, and never blocks. Any run may rewrite files, so the content hash and the files read
// after it are those of a new epoch.
func (r *answerHookRun) runEntry(e workflow.Entry, ans agent.Answer) (agent.Answer, bool) {
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
			r.log(workflow.VerbRun, e.Line, "skip", key+" ya pasó en este contenido")
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
	if slices.Contains(workflow.Informed, r.ev.Event.Name) {
		r.log(workflow.VerbRun, e.Line, "inform", reason)
		return ans.Inform(reason), false
	}
	return r.block(e, ans, reason, key)
}

// runKey returns the arguments of the command of run, expanded, and the key of its repeat: those arguments joined
// by single spaces, or for the checks of the rules the word rules and the expanded target. A {path} in the command
// or the target makes the key differ per path.
func (r *answerHookRun) runKey(run workflow.Run) ([]string, string) {
	if run.Rules != "" {
		return nil, "rules " + r.expand(run.Rules)
	}
	args := make([]string, len(run.Command))
	for i, a := range run.Command {
		args[i] = r.expand(a)
	}
	return args, strings.Join(args, " ")
}

// passed reports whether a run of key passed on the current branch and the content hash. A log that cannot be read
// never counts as passed: the run runs again.
func (r *answerHookRun) passed(key, hash string) bool {
	branch := r.branch()
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomePassed, Branch: branch, ByBranch: true, Key: key, Hash: hash, ByHash: true}
	if q.Valid() != nil {
		return false
	}
	return st.Index(q) >= 0
}

// observeFinished writes that the subagent of a subagent_stop the answer does not refuse finished, its type as the
// key: with the content hash when a `finished` of the workflow names the type, so the condition can hold; without
// it, and without computing it, when the workflow only injects once, so the injects of that subagent are due again.
// Without a known hash, whose cause the facts already keep as a warning, the condition gets nothing, with a warn
// decision, and the line is written without hash only when the workflow injects once. A stop the loop guard let
// through did not end as asked: it is written without hash, only when the workflow injects once, so the subagent
// never counts as finished but its next run, with a new context, gets the injects again.
func (r *answerHookRun) observeFinished() {
	ev := r.ev.Event
	if r.guarded {
		if r.ev.Workflow.InjectsOnce() {
			_ = r.write(state.KindObserved, "", 0, state.OutcomeFinished, "", ev.Actor, "")
		}
		return
	}
	observed := false
	for _, name := range r.ev.Workflow.ObservedSubagents() {
		observed = observed || r.expand(name) == ev.Actor
	}
	if r.waiting() {
		return
	}
	once := r.ev.Workflow.InjectsOnce()
	hash := ""
	if observed {
		h, known := r.hash()
		if r.waiting() {
			return
		}
		if !known {
			r.log("finished", 0, "warn", "hash del contenido desconocido; no se registra que "+ev.Actor+
				" terminó para finished")
		}
		hash = h
	}
	if hash != "" || once {
		_ = r.write(state.KindObserved, "", 0, state.OutcomeFinished, "", ev.Actor, hash)
	}
}

// finished reports whether the subagent type name finished on the current branch on a content whose files in scope
// did not change since: the tree of its latest observed finished with a hash is compared with the current one (see
// sameScope). The log is read first, so without such a line the content hash is never computed. A log that cannot be
// read, with a warning, an unknown hash, a tree git no longer has or a comparison that fails never holds, the last
// two with the error in the log.
func (r *answerHookRun) finished(name string, scope []string) bool {
	branch := r.branch()
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomeFinished, Branch: branch, ByBranch: true, Key: name}
	if q.Valid() != nil {
		return false
	}
	var last state.Entry
	for _, e := range st.Observed(q) {
		if e.Hash != "" {
			last = e
		}
	}
	if last.Hash == "" {
		return false
	}
	hash, known := r.hash()
	if r.waiting() || !known {
		return false
	}
	same, err := r.sameScope(last.Hash, hash, scope)
	if err != nil {
		r.log("finished", r.entry.Line, "warn", fmt.Sprintf(
			"no se puede comparar el contenido en que terminó %s con el actual (%v): finished no se cumple", name, err))
		return false
	}
	return same
}

// sameScope reports whether the files in scope are the same in the trees from and to: the trees are equal, or no
// path that differs between them matches the globs of scope; with no scope, any differing path counts. It asks for
// the comparison it lacks, and returns why the trees could not be compared (a tree git no longer has, or git
// failing): they then count as different.
func (r *answerHookRun) sameScope(from, to string, scope []string) (bool, error) {
	if from == to {
		return true, nil
	}
	paths, compared, err := r.ev.Facts.TreeDiff(from, to)
	if !compared {
		s := Step{Compare: from, Tree: to}
		r.ask(s, s.Valid())
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, p := range paths {
		if len(scope) == 0 || glob.AnyPath(scope, p) {
			return false, nil
		}
	}
	return true, nil
}

// run executes the command of run, with its expanded args, or the checks of the rules and returns why it failed,
// or "".
func (r *answerHookRun) run(run workflow.Run, args []string) string {
	if run.Rules != "" {
		return r.runRules(run.Rules)
	}
	s := Step{Run: args}
	o := r.act(s, s.Valid())
	if r.waiting() || o.Err == nil {
		return ""
	}
	return fmt.Sprintf("Falla %s: %v\n%s", strings.Join(args, " "), o.Err,
		Indent(text.LastLines(o.Output, answerHookOutputLines, rule.MaxLineLen)))
}

// runRules runs the checks of the rules that apply to the changed files (`changed`) or to a path, through the
// runners of rules.run that match their files or as programs, grouped as the runners group them (rule.Plan); a
// failed shared run is rerun per rule (Run.Settle). It returns the rules that failed, or "".
func (r *answerHookRun) runRules(target string) string {
	all, err := r.rules()
	if r.waiting() {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("No se pueden cargar las rules: %v", err)
	}
	var paths []string
	if target == "changed" {
		paths = r.changed()
	} else {
		paths = []string{r.expand(target)}
	}
	var results []rule.Result
	queue := rule.Plan(rule.Matching(all, paths), r.ev.Workflow)
	for len(queue) > 0 {
		run := queue[0]
		s := Step{Check: &run}
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
	failed := rule.Failed(results)
	if len(failed) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Rules que no se cumplen:")
	for _, f := range failed {
		fmt.Fprintf(&b, "\n- %s (%s): %s\n%s", f.Rule.ID, f.Rule.Check, f.Rule.Description, Indent(f.Tail()))
	}
	return b.String()
}

// inject carries out the verb inject of the entry e: the text, the file or the rules that apply to a path. On
// start, which always injects, or with always, it injects every time it applies. Otherwise it injects
// once: keyed by the session, the actor, the line of the entry and the expanded text or file path (one key per rule
// id for the rules), it is skipped while not due (see due) and, once the answer shows it reaches the AI, an observed
// `injected` line is written for it (see writeInjected). A placeholder that changes on every event makes a new key,
// so it injects every time.
func (r *answerHookRun) inject(e workflow.Entry, ans agent.Answer) agent.Answer {
	in := *e.Action.Inject
	once := !in.Always && r.ev.Event.Name != workflow.Start
	if in.Rules != "" {
		target := r.expand(in.Rules)
		if r.waiting() {
			return ans
		}
		if t := r.contextRules(e.Line, target, once); t != "" && !r.waiting() {
			ans = ans.Inform(t)
		}
		return ans
	}
	key := in.Text
	if in.File != "" {
		key = in.File
	}
	key = r.expand(key)
	if r.waiting() || key == "" {
		return ans // a file whose name expands to nothing names no file
	}
	if once {
		due := r.due(e.Line, key)
		if r.waiting() {
			return ans
		}
		if !due {
			r.log(workflow.VerbInject, e.Line, "skip", "ya se inyectó a "+r.ev.Event.Actor+" en esta sesión")
			return ans
		}
	}
	text := key
	if in.File != "" {
		data, readable := r.read(key)
		if !readable {
			return ans
		}
		text = data
	}
	if text == "" {
		return ans
	}
	if once {
		r.injected = append(r.injected, answerHookInjected{line: e.Line, key: key})
	}
	return ans.Inform(text)
}

// writeInjected writes an observed `injected` line, without hash, for each once-only inject that reached the answer
// ans, in the order they were injected. On prompt a refusal rejects the prompt, so its contexts never reach the AI
// and nothing is written; on the other events the contexts reach the AI with the block, and they are written.
func (r *answerHookRun) writeInjected(ans agent.Answer) {
	if r.ev.Event.Name == workflow.Prompt && ans.Block {
		return
	}
	for _, in := range r.injected {
		_ = r.write(state.KindObserved, "", in.line, state.OutcomeInjected, "", in.key, "")
		if r.waiting() {
			return
		}
	}
}

// contextRules lists the rules that apply to target; with once, only those due for the actor of the event (see
// due), each kept to be written as injected with its id as the key (see writeInjected), and the skip is logged when
// every rule that applies was already injected.
func (r *answerHookRun) contextRules(line int, target string, once bool) string {
	all, _ := r.rules()
	if r.waiting() {
		return ""
	}
	matching := rule.Matching(all, []string{target})
	var lines []string
	for _, rl := range matching {
		if once {
			due := r.due(line, rl.ID)
			if r.waiting() {
				return ""
			}
			if !due {
				continue
			}
			r.injected = append(r.injected, answerHookInjected{line: line, key: rl.ID})
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", rl.ID, rl.Description))
	}
	if len(lines) == 0 {
		if len(matching) > 0 {
			r.log(workflow.VerbInject, line, "skip", "las rules de "+target+" ya se inyectaron a "+r.ev.Event.Actor+
				" en esta sesión")
		}
		return ""
	}
	return fmt.Sprintf("Rules that apply to %s (%s/):\n%s", target, r.ev.Workflow.RulesPath(),
		strings.Join(lines, "\n"))
}

// due reports whether a once-only inject of the entry at line with key is due for the actor of the event: it never
// reached the actor in the session, or after it reached it a start of the session (a new or compacted
// context) or, for a subagent, the end of a subagent of its type in the session (its next run starts with a new
// context) was observed. A log that cannot be read makes it due every time, with a warning.
func (r *answerHookRun) due(line int, key string) bool {
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return true
	}
	ev := r.ev.Event
	injected := state.Query{Outcome: state.OutcomeInjected, Session: ev.Session, Actor: ev.Actor, Line: line, Key: key}
	started := state.Query{Outcome: state.OutcomeStarted, Session: ev.Session}
	finished := state.Query{Outcome: state.OutcomeFinished, Session: ev.Session, Key: ev.Actor}
	if injected.Valid() != nil || started.Valid() != nil || finished.Valid() != nil {
		return true
	}
	last := st.Index(injected)
	if last < 0 {
		return true
	}
	reset := st.Index(started)
	if ev.Actor != agent.ActorSession {
		reset = max(reset, st.Index(finished))
	}
	return reset > last
}

// warnLog warns the user, once per pass, that the log cannot be read: the verbs that repeat by it act every time, and
// finished never holds.
func (r *answerHookRun) warnLog(err error) {
	w := fmt.Sprintf("polix no puede usar su log (%v): run, inject y los block con once actúan cada vez, y finished "+
		"no se cumple", err)
	if !slices.Contains(r.warnings, w) {
		r.warnings = append(r.warnings, w)
	}
}

// matches reports whether every condition of c holds for the event, cheapest first, then in the order of their
// names. scope holds the globs of the nearest `changed` of the enclosing maps, which the measures take; a
// `changed` of c replaces it for c and the maps nested in it.
func (r *answerHookRun) matches(c workflow.Cond, scope []string) bool {
	if v, ok := c["changed"]; ok {
		scope = r.list(v)
	}
	for _, k := range answerHookKeys(c) {
		if !r.condition(k, c[k], c, scope) || r.waiting() {
			return false
		}
	}
	return true
}

// answerHookKeys returns the conditions of c, cheapest first, then in the order of their names.
func answerHookKeys(c workflow.Cond) []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := answerHookCost(keys[i]), answerHookCost(keys[j])
		return ri < rj || ri == rj && keys[i] < keys[j]
	})
	return keys
}

// answerHookCost ranks a condition by what it reads: the event first, then what git or the log give, so a cheap
// condition that fails spares the content hash.
func answerHookCost(k string) int {
	switch k {
	case "path", "actor", "command", "exit", "contains":
		return 0
	case "writes":
		return 1
	case "branch", "var":
		return 2
	case "changed", "file":
		return 3
	case "changed_lines", "changed_files", "risk_score":
		return 4
	case "finished":
		return 5
	}
	return 6 // not and any, which may read anything
}

// condition evaluates one condition of all; scope holds the globs the measures take (see matches).
func (r *answerHookRun) condition(k string, v any, all workflow.Cond, scope []string) bool {
	ev := r.ev.Event
	switch k {
	case "not":
		nested, _ := workflow.Nested(v)
		m := r.matches(nested, scope)
		return !r.waiting() && !m
	case "any":
		nested, _ := workflow.Nested(v)
		// The `changed` of the nested map scopes its measures, as in matches.
		if cv, ok := nested["changed"]; ok {
			scope = r.list(cv)
		}
		for _, nk := range answerHookKeys(nested) {
			holds := r.condition(nk, nested[nk], nested, scope)
			if r.waiting() {
				return false
			}
			if holds {
				return true
			}
		}
		return false
	case "changed_lines", "changed_files", "risk_score":
		m, err := workflow.Measured(v)
		if err != nil {
			return false
		}
		d, measured := r.diff()
		if r.waiting() {
			return false
		}
		if !measured {
			return true // fails closed: a diff that cannot be measured never skips what the measure guards
		}
		return Measure(k, m, d.Narrow(scope), r.ev.Workflow.Weights)
	case "branch":
		s, _ := v.(string)
		return Branch(s, r.branch())
	case "var":
		vars := Vars(r.ev.Workflow, r.branch())
		for _, name := range r.list(v) {
			if vars[name] == "" {
				return false
			}
		}
		return true
	case "path":
		// Without case where file names ignore it, as writes: `.ENV` is `.env` there.
		return ev.Path != "" && AnyPathFold(r.list(v), ev.Path, r.ev.Fold())
	case "changed":
		patterns := r.list(v)
		for _, f := range r.changed() {
			if glob.AnyPath(patterns, f) {
				return true
			}
		}
		return false
	case "file":
		return r.fileExists(r.list(v), all["contains"])
	case "contains":
		return true // evaluated together with file
	case "command":
		return len(MatchingCommands(r.list(v), ev.Command)) > 0
	case "writes":
		_, _, ok := r.writes(r.list(v))
		return ok
	case "exit":
		n, _ := v.(int)
		// The exit code of the line only speaks for a command whose failure propagates to it.
		if patterns, ok := all["command"]; ok && !Propagates(MatchingCommands(r.list(patterns), ev.Command)) {
			return false
		}
		return ev.ExitCode == n
	case "actor":
		return slices.Contains(r.list(v), ev.Actor)
	case "finished":
		// The content finished compares is the scope of the entry: the nearest changed, else the one of its when.
		if scope == nil {
			scope = r.whenScope()
		}
		for _, name := range r.list(v) {
			if !r.finished(name, scope) || r.waiting() {
				return false
			}
		}
		return true
	}
	return false
}

// list reads the value of a condition as a list of strings, expanded.
func (r *answerHookRun) list(v any) []string {
	s, _ := workflow.Strings(v)
	out := make([]string, len(s))
	for i := range s {
		out[i] = r.expand(s[i])
	}
	return out
}

// fileExists reports whether a file of the repository matches one of the patterns and, with contains, holds
// that text.
func (r *answerHookRun) fileExists(patterns []string, contains any) bool {
	want := ""
	if s, ok := contains.(string); ok {
		want = r.expand(s)
	}
	if want != "" {
		return r.contains(patterns, want)
	}
	for _, f := range r.files() {
		if glob.AnyPath(patterns, f) {
			return true
		}
	}
	return false
}

// contains reports whether a file matching the patterns holds want, at this epoch.
func (r *answerHookRun) contains(patterns []string, want string) bool {
	if r.waiting() {
		return false
	}
	if found, known := r.ev.Facts.Found(r.epoch, patterns, want); known {
		return found
	}
	s := Step{Contains: patterns, Text: want, Epoch: r.epoch}
	r.ask(s, s.Valid())
	return false
}

// writes returns the first path a command of the event writes that matches one of the patterns (see Writes).
func (r *answerHookRun) writes(patterns []string) (p, held string, ok bool) {
	if len(patterns) == 0 {
		return "", "", false
	}
	ev := r.ev.Event
	paths := append([]string{r.ev.Root}, WritePaths(ev.Command, ev.Cwd, r.ev.Root)...)
	reals := r.reals(paths...)
	if r.waiting() {
		return "", "", false
	}
	return Writes(patterns, ev.Command, ev.Cwd, r.ev.Root, reals, r.ev.Fold(), r.ev.Windows())
}

// expand replaces {var}, {path}, {command}, {branch} and the measures in s. The diff is read only when s holds a
// measure placeholder, and measured over the globs of the `changed` of the `when` of the entry, else whole.
func (r *answerHookRun) expand(s string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	branch := r.branch()
	vars := Vars(r.ev.Workflow, branch)
	var measures map[string]string
	if UsesMeasures(s) {
		d, measured := r.diff()
		scope := r.whenScope()
		if r.waiting() {
			return s
		}
		measures = Measures(d.Narrow(scope), r.ev.Workflow.Weights, measured)
	}
	return Expand(s, r.ev.Event, branch, vars, measures)
}

// whenScope returns the globs of the `changed` of the `when` of the entry, expanded without the measures, so a glob
// never asks for the diff it scopes; nil without one.
func (r *answerHookRun) whenScope() []string {
	globs, _ := workflow.Strings(r.entry.When["changed"])
	if len(globs) == 0 {
		return nil
	}
	branch := r.branch()
	vars := Vars(r.ev.Workflow, branch)
	scope := make([]string, len(globs))
	for i, g := range globs {
		scope[i] = Expand(g, r.ev.Event, branch, vars, nil)
	}
	return scope
}

// diff returns the size of the change of each file not committed yet, and whether it could be measured.
func (r *answerHookRun) diff() (diff.Diff, bool) {
	if r.ev.Facts.HasDiff {
		return r.ev.Facts.Diff, r.ev.Facts.DiffErr == nil
	}
	s := Step{Diff: true}
	r.ask(s, s.Valid())
	return diff.Diff{}, false
}

// log records a decision on the event.
func (r *answerHookRun) log(action string, line int, outcome, reason string) {
	_ = r.write(state.KindDecision, action, line, outcome, reason, "", "")
}

// write appends to the log an entry of kind about the event and returns the error of appending it. The content
// hash costs a git call over the whole tree, so a decision only carries the one this call already computed; an
// observed entry carries the hash it is given, none for those that are not about a content (injected, started and
// a finished that only makes injects due again).
func (r *answerHookRun) write(kind, action string, line int, outcome, reason, key, hash string) error {
	ev := r.ev.Event
	branch := r.branch()
	if r.waiting() {
		return nil
	}
	if hash == "" && kind == state.KindDecision {
		hash, _, _ = r.ev.Facts.Hash(r.epoch)
	}
	e := state.Entry{Kind: kind, Agent: ev.Agent, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
		Branch: branch, Hash: hash, Action: action, Line: line, Key: key, Outcome: outcome, Reason: reason}
	return r.appendEntry(e, e.Valid())
}

// appendEntry appends e to the log and returns the error of appending it. Once the log is read, an entry
// appended afterwards is added to it as well, so a later entry of the call sees it.
func (r *answerHookRun) appendEntry(e state.Entry, valid error) error {
	if valid != nil {
		return valid
	}
	s := Step{Append: &e}
	o := r.act(s, s.Valid())
	if r.waiting() {
		return nil
	}
	f := r.ev.Facts
	if o.Err == nil && f.HasState && f.StateErr == nil && r.next > f.StateAt {
		r.state, _ = r.state.Append(e)
	}
	return o.Err
}

// branch returns the current branch.
func (r *answerHookRun) branch() string {
	if r.ev.Facts.HasBranch {
		return r.ev.Facts.Branch
	}
	s := Step{Branch: true}
	r.ask(s, s.Valid())
	return ""
}

// hash returns the content hash of this epoch and whether it is known.
func (r *answerHookRun) hash() (string, bool) {
	if h, known, computed := r.ev.Facts.Hash(r.epoch); computed {
		return h, known
	}
	s := Step{Hash: true, Epoch: r.epoch}
	r.ask(s, s.Valid())
	return "", false
}

// changed returns the files not committed yet.
func (r *answerHookRun) changed() []string {
	if r.ev.Facts.HasChanged {
		return r.ev.Facts.Changed
	}
	s := Step{Changed: true}
	r.ask(s, s.Valid())
	return nil
}

// files returns the files of the working tree git does not ignore.
func (r *answerHookRun) files() []string {
	if r.ev.Facts.HasFiles {
		return r.ev.Facts.Files
	}
	s := Step{Files: true}
	r.ask(s, s.Valid())
	return nil
}

// rules returns the rules of the repository, or why they cannot be loaded.
func (r *answerHookRun) rules() ([]rule.Rule, error) {
	if r.ev.Facts.HasRules {
		return r.ev.Facts.Rules, r.ev.Facts.RulesErr
	}
	s := Step{Rules: true}
	r.ask(s, s.Valid())
	return nil, nil
}

// currentState returns the log as this pass left it, or why it cannot be read (then it is empty).
func (r *answerHookRun) currentState() (state.State, error) {
	if r.ev.Facts.HasState {
		return r.state, r.ev.Facts.StateErr
	}
	s := Step{State: true}
	r.ask(s, s.Valid())
	return state.State{}, nil
}

// ignored reports whether git ignores the path.
func (r *answerHookRun) ignored(p string) bool {
	if ignored, ok := r.ev.Facts.Ignored[p]; ok {
		return ignored
	}
	s := Step{Ignored: p}
	r.ask(s, s.Valid())
	return false
}

// reals returns the real paths of the paths, asking for all those not known yet at once.
func (r *answerHookRun) reals(paths ...string) map[string]string {
	var missing []string
	for _, p := range paths {
		if _, ok := r.ev.Facts.Reals[p]; !ok && !slices.Contains(missing, p) {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		s := Step{Real: missing}
		r.ask(s, s.Valid())
		return nil
	}
	return r.ev.Facts.Reals
}

// read returns the content of the file of this epoch and whether it can be read.
func (r *answerHookRun) read(p string) (string, bool) {
	if r.waiting() || p == "" {
		return "", false // an empty path names no file: it cannot be read
	}
	if data, readable, read := r.ev.Facts.Content(r.epoch, p); read {
		return data, readable
	}
	s := Step{Read: p, Epoch: r.epoch}
	r.ask(s, s.Valid())
	return "", false
}
