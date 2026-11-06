package hook

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/mlophez/polix-ai/internal/agent"
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
}

// answerHookOutputLines is how many lines of the output of a failing command its reason keeps, the last ones.
const answerHookOutputLines = 15

// AnswerHook decides the next step of the evaluation of an event: the protections that come before the workflow
// (nobody edits the state of polix, no actor writes a protected path), then, with a valid workflow, its entries
// in order, each with its conditions and its action. It walks the event from its start every time it is asked,
// so the step it returns is the first one whose result the evaluation does not know yet; with none, the event is
// answered.
func AnswerHook(ev Evaluation) AnswerHookOutput {
	r := &answerHookRun{ev: ev, epoch: ev.Facts.Epoch, state: ev.Facts.State}
	ans := r.handle()
	if r.step != nil {
		return AnswerHookOutput{Step: *r.step}
	}
	return AnswerHookOutput{Answer: ans, Facts: ev.Facts.Carry(r.epoch, r.state)}
}

// handle answers the event: the protections, then the entries of the event in order. It stops early when a step
// is missing; the answer is then meaningless.
func (r *answerHookRun) handle() agent.Answer {
	ans, blocked := r.protect()
	if r.waiting() {
		return ans
	}
	if blocked {
		r.log("builtin", 0, "deny", strings.Join(ans.Reasons, "; "))
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
	if r.ev.Event.Name == workflow.SessionStart {
		if text := r.ev.Workflow.SessionContext(); text != "" {
			ans = ans.Inform(text)
		}
	}
	for _, e := range r.ev.Workflow.On[r.ev.Event.Name] {
		if !r.matches(e.If) || len(e.Unless) > 0 && r.matches(e.Unless) {
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
	for _, w := range r.ev.Facts.Warnings {
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

// protect applies the protections that come before the workflow: nobody edits the state of polix, whatever the
// workflow says, and no actor, the main session included, writes a path of the protected list of the workflow,
// also when git ignores it. Reading is always allowed.
func (r *answerHookRun) protect() (agent.Answer, bool) {
	ev, fold := r.ev.Event, r.ev.Fold()
	var protected []string
	if r.ev.Invalid == nil && r.ev.Present {
		protected = r.ev.Workflow.Protected
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
				return agent.Answer{}.Refuse(ReasonStateEdit), true
			}
		}
		if !ev.Outside && ev.Path != "" && AnyPathFold(protected, ev.Path, fold) {
			return agent.Answer{}.Refuse(fmt.Sprintf("%s is protected in %s: only a person changes it.", ev.Path,
				workflow.File)), true
		}
	case workflow.PreCommand, workflow.PreCommit:
		// Reading is fine (the log is the evidence of the process); only commands that could write are denied.
		if MayWrite(ev.Command) {
			reals := r.reals(r.ev.StateDir())
			if r.waiting() {
				return agent.Answer{}, false
			}
			if NamesState(ev.Command, r.ev.StateDir(), reals[r.ev.StateDir()], fold) {
				return agent.Answer{}.Refuse(ReasonStateCommand), true
			}
		}
		if p, held, ok := r.writes(protected); ok {
			if held != "" {
				return agent.Answer{}.Refuse(fmt.Sprintf("%s holds %s, protected in %s: only a person changes it.",
					p, held, workflow.File)), true
			}
			return agent.Answer{}.Refuse(fmt.Sprintf("%s is protected in %s: only a person changes it.", p,
				workflow.File)), true
		}
	}
	return agent.Answer{}, false
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
	reason := "polix: el workflow no es válido; corrígelo y ejecuta polix check.\n" + problems
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

// apply runs the action of a matching entry and reports whether the evaluation of the event stops there. Once the
// action applied, the fact of the entry is recorded (recordEntry), before the evaluation stops when it does; a run
// records it itself, only when it passed (runEntry).
func (r *answerHookRun) apply(e workflow.Entry, ans agent.Answer) (agent.Answer, bool) {
	switch e.Action() {
	case "deny":
		// Only a block that happened stops the evaluation; a warning, an audit or the loop guard let it go on.
		reason := r.expand(*e.Deny)
		if r.waiting() {
			return ans, false
		}
		ans, stop := r.block(e, ans, reason)
		r.recordEntry(e)
		return ans, stop
	case "allow":
		r.log("allow", e.Line, "allow", "")
		r.recordEntry(e)
		return ans, true
	case "require":
		reason := r.expand(*e.Require)
		if r.waiting() {
			return ans, false
		}
		ans, _ = r.block(e, ans, reason)
		r.recordEntry(e)
		return ans, false
	case "run":
		return r.runEntry(e, ans), false
	case "context":
		if t := r.context(*e.Context); t != "" && !r.waiting() {
			ans = ans.Inform(t)
		}
		r.recordEntry(e)
		return ans, false
	case "record":
		r.recordEntry(e)
		return ans, false
	}
	return ans, false
}

// recordEntry records the fact of the entry e, if it has one, on the current content: its action applied, blocked,
// warned or let through by the loop guard. An entry in audit mode records nothing, so trying an entry never
// changes what the others see.
func (r *answerHookRun) recordEntry(e workflow.Entry) {
	if e.Record == nil || e.EffectiveMode() == workflow.ModeAudit {
		return
	}
	fact := r.expand(*e.Record)
	if r.waiting() {
		return
	}
	r.recordFact(fact)
}

// block applies a block of the entry e in its mode and reports whether the event is actually blocked. On stop and
// subagent_stop, the loop guard of every block (require, deny or a failing run): after max_stop_blocks blocks of
// this entry on this same content, the agent may stop and a trace is left. An unknown hash counts as one content.
// When the log cannot be read or written the guard cannot count, so it fails open.
func (r *answerHookRun) block(e workflow.Entry, ans agent.Answer, reason string) (agent.Answer, bool) {
	ev := r.ev.Event
	switch e.EffectiveMode() {
	case workflow.ModeAudit:
		r.log(e.Action(), e.Line, "audit", reason)
		return ans, false
	case workflow.ModeWarn:
		ans = ans.Inform("Aviso de polix: " + reason).Warn(reason)
		r.log(e.Action(), e.Line, "warn", reason)
		return ans, false
	}
	if ev.Name == workflow.Stop || ev.Name == workflow.SubagentStop {
		hash, _ := r.hash()
		st, err := r.currentState()
		if r.waiting() {
			return ans, false
		}
		if n := st.StopBlocks(ev.Session, ev.Name, ev.Actor, e.Line, hash); err == nil && n >= r.ev.Workflow.MaxStopBlocks {
			r.log(e.Action(), e.Line, "loop_guard", reason)
			return ans.Warn(fmt.Sprintf("polix deja parar tras %d bloqueos: %s", n, reason)), false
		}
		if err == nil {
			err = r.write(state.KindStopBlock, "", e.Line, "", reason, "", hash)
			if r.waiting() {
				return ans, false
			}
		}
		if err != nil {
			r.log(e.Action(), e.Line, "loop_guard", reason)
			return ans.Warn(fmt.Sprintf("polix deja parar porque no puede usar su log (%v): %s", err, reason)), false
		}
	}
	r.log(e.Action(), e.Line, "block", reason)
	return ans.Refuse(reason), true
}

// runEntry runs the action run of the entry e. The fact is about the content the run saw: it is pinned before
// running, and nothing is recorded if the run (or anyone else meanwhile) changed it, or if the hash is unknown.
// Only a run that passed records, and never in audit mode. Any run may rewrite files, so the content hash and the
// files read after it are those of a new epoch.
func (r *answerHookRun) runEntry(e workflow.Entry, ans agent.Answer) agent.Answer {
	records := e.Record != nil && e.EffectiveMode() != workflow.ModeAudit
	before, knownBefore := "", true
	if records {
		before, knownBefore = r.hash()
	}
	reason := r.run(*e.Run)
	if r.waiting() {
		return ans
	}
	r.epoch++
	if reason != "" {
		ans, _ = r.block(e, ans, reason)
		return ans
	}
	if !records {
		return ans
	}
	fact := r.expand(*e.Record)
	after, knownAfter := r.hash()
	if r.waiting() {
		return ans
	}
	switch {
	case !knownBefore || !knownAfter:
		r.log("run", e.Line, "warn", "hash del contenido desconocido; no se registra "+fact)
	case after != before:
		reason := fmt.Sprintf("el contenido cambió durante el run; no se registra %s", fact)
		ans = ans.Warn(reason)
		r.log("run", e.Line, "warn", reason)
	default:
		r.recordFact(fact)
	}
	return ans
}

// run executes a command or the checks of the rules and returns why it failed, or "".
func (r *answerHookRun) run(run workflow.Run) string {
	if run.Rules != "" {
		return r.runRules(run.Rules)
	}
	args := make([]string, len(run.Command))
	for i, a := range run.Command {
		args[i] = r.expand(a)
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

// context builds the text of a context action; the rules are injected once per session.
func (r *answerHookRun) context(c workflow.Context) string {
	switch {
	case c.File != "":
		data, readable := r.read(r.expand(c.File))
		if !readable {
			return ""
		}
		return data
	case c.Text != "":
		return r.expand(c.Text)
	case c.Rules != "":
		return r.contextRules(r.expand(c.Rules))
	}
	return ""
}

// contextRules lists the rules that apply to target and were not injected in the session yet, and records that
// they were.
func (r *answerHookRun) contextRules(target string) string {
	all, _ := r.rules()
	if r.waiting() {
		return ""
	}
	ev := r.ev.Event
	var lines []string
	for _, rl := range rule.Matching(all, []string{target}) {
		st, _ := r.currentState()
		if r.waiting() {
			return ""
		}
		if st.Injected(rl.ID, ev.Session) {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", rl.ID, rl.Description))
		e := state.Entry{Kind: state.KindInjected, Agent: ev.Agent, Session: ev.Session, Rule: rl.ID}
		_ = r.appendEntry(e, e.Valid())
		if r.waiting() {
			return ""
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf("Rules that apply to %s (%s/):\n%s", target, r.ev.Workflow.RulesPath(),
		strings.Join(lines, "\n"))
}

// matches reports whether every condition of c holds for the event, in the order of their names.
func (r *answerHookRun) matches(c workflow.Cond) bool {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, rj := answerHookCost(keys[i]), answerHookCost(keys[j])
		return ri < rj || ri == rj && keys[i] < keys[j]
	})
	for _, k := range keys {
		if !r.condition(k, c[k], c) || r.waiting() {
			return false
		}
	}
	return true
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
	case "done":
		return 4
	}
	return 5 // not, which may read anything
}

// condition evaluates one condition of all.
func (r *answerHookRun) condition(k string, v any, all workflow.Cond) bool {
	ev := r.ev.Event
	switch k {
	case "not":
		nested, _ := workflow.Nested(v)
		m := r.matches(nested)
		return !r.waiting() && !m
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
		return ev.Path != "" && glob.AnyPath(r.list(v), ev.Path)
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
	case "done":
		for _, fact := range r.list(v) {
			hash, known := r.hash()
			st, _ := r.currentState()
			if r.waiting() || !known || !st.Done(fact, r.branch(), hash) {
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

// expand replaces {var}, {path}, {command} and {branch} in s.
func (r *answerHookRun) expand(s string) string {
	if !strings.Contains(s, "{") {
		return s
	}
	branch := r.branch()
	return Expand(s, r.ev.Event, branch, Vars(r.ev.Workflow, branch))
}

// recordFact records fact on the current content; nothing without a known hash, whose cause the facts already
// keep as a warning.
func (r *answerHookRun) recordFact(fact string) {
	hash, known := r.hash()
	if r.waiting() {
		return
	}
	if !known {
		r.log("record", 0, "warn", "hash del contenido desconocido; no se registra "+fact)
		return
	}
	_ = r.write(state.KindFact, "", 0, "", "", fact, hash)
}

// log records a decision on the event.
func (r *answerHookRun) log(action string, line int, outcome, reason string) {
	_ = r.write(state.KindDecision, action, line, outcome, reason, "", "")
}

// write appends to the log an entry of kind about the event and returns the error of appending it. The content
// hash costs a git call over the whole tree, so an entry without its own hash only carries the one this call
// already computed; facts and stop blocks, which depend on it, give theirs.
func (r *answerHookRun) write(kind, action string, line int, outcome, reason, fact, hash string) error {
	ev := r.ev.Event
	branch := r.branch()
	if r.waiting() {
		return nil
	}
	if hash == "" {
		hash, _, _ = r.ev.Facts.Hash(r.epoch)
	}
	e := state.Entry{Kind: kind, Agent: ev.Agent, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
		Branch: branch, Hash: hash, Fact: fact, Action: action, Line: line, Outcome: outcome, Reason: reason}
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
