// Package engine evaluates the workflow of a repository for one neutral event and decides the answer: block
// it with a reason, let it pass, inject context, run commands or rule checks, and record facts.
//
// It measures and never judges: conditions are facts of the repository, the event and the state; judging the
// code is the job of the reviewer subagent.
package engine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/gitx"
	"github.com/mlophez/polix-ai/internal/glob"
	"github.com/mlophez/polix-ai/internal/rules"
	"github.com/mlophez/polix-ai/internal/shell"
	"github.com/mlophez/polix-ai/internal/state"
	"github.com/mlophez/polix-ai/internal/workflow"
)

// RunBudget bounds every `run` of one hook call together (commands and rule checks). The timeout of the hook in
// the agent settings must be longer, or the agent kills the hook first and the gate is skipped.
const RunBudget = 10 * time.Minute

// Actor of the main session; subagents use their own type (builder, reviewer...).
const ActorSession = "session"

// Event is what an adapter makes of a hook call: the same shape for every agent.
type Event struct {
	Name     string // a neutral event of the workflow package
	Agent    string // claude, codex
	Session  string
	Actor    string // ActorSession or the subagent type
	Path     string // pre_edit and post_edit: repository relative, slash separated
	AbsPath  string // pre_edit and post_edit: absolute path as the agent gave it
	Outside  bool   // the edited path is outside the repository
	Command  string // pre_command, post_command, pre_commit
	ExitCode int    // post_command
	Prompt   string // prompt
	Cwd      string
}

// Result is the answer to one or more events.
type Result struct {
	Block    bool
	Reasons  []string // why it is blocked, or what is required
	Contexts []string // text to inject into the session
	Warnings []string // messages for the user that do not block
}

// Merge adds another result to r, skipping texts it already has: the events of one call (an edit of several
// paths, a command and its commit) often give the same message.
func (r *Result) Merge(o Result) {
	r.Block = r.Block || o.Block
	r.Reasons = appendNew(r.Reasons, o.Reasons)
	r.Contexts = appendNew(r.Contexts, o.Contexts)
	r.Warnings = appendNew(r.Warnings, o.Warnings)
}

func appendNew(list, more []string) []string {
	for _, s := range more {
		if !slices.Contains(list, s) {
			list = append(list, s)
		}
	}
	return list
}

// Engine holds what the evaluation of one hook call needs; repository facts are read lazily and once.
type Engine struct {
	Repo     *gitx.Repo
	Workflow *workflow.Workflow // nil when the repository has no workflow
	Invalid  error              // the problems of the workflow when it is not valid; Workflow is then ignored
	Store    *state.Store

	deadline  time.Time // end of the RunBudget of this call
	branch    *string
	hash      *string
	changed   []string
	gotChange bool
	files     []string
	rules     []rules.Rule
	rulesErr  error
	gotRules  bool
	vars      map[string]string
}

// New prepares an engine for repo.
func New(repo *gitx.Repo, wf *workflow.Workflow) *Engine {
	return &Engine{Repo: repo, Workflow: wf, Store: state.Open(repo.GitDir), deadline: time.Now().Add(RunBudget)}
}

// Handle evaluates the built-in protections and then the entries of the event, in order.
func (en *Engine) Handle(ev Event) Result {
	if res, blocked := en.protect(ev); blocked {
		en.log(ev, "builtin", 0, "deny", strings.Join(res.Reasons, "; "))
		return res
	}
	if en.Invalid != nil {
		return en.invalid(ev)
	}
	var res Result
	if en.Workflow == nil || en.outsideChange(ev) {
		return res
	}
	for _, e := range en.Workflow.On[ev.Name] {
		if !en.matches(e.If, ev) || (len(e.Unless) > 0 && en.matches(e.Unless, ev)) {
			continue
		}
		stop := en.apply(&e, ev, &res)
		if stop {
			break
		}
	}
	return res
}

// outsideChange reports an edit outside the repository (scratchpad, memory, another repository) or of a file
// git ignores: it is not part of the change, so the workflow does not apply to it.
func (en *Engine) outsideChange(ev Event) bool {
	return (ev.Name == workflow.PreEdit || ev.Name == workflow.PostEdit) && (ev.Outside || en.Repo.Ignored(ev.Path))
}

// invalid answers an event when the workflow is not valid. Edits and commands fail closed, so a broken file is
// noticed at once instead of silently disabling the workflow, except what fixing it needs: the main session may
// edit the workflow and run `polix check`, and edits outside the change never depended on the workflow. Other
// events only warn: a blocked stop would keep the agent in a loop it cannot leave.
func (en *Engine) invalid(ev Event) Result {
	problems := en.Invalid.Error()
	switch ev.Name {
	case workflow.PreEdit:
		if en.outsideChange(ev) || (ev.Actor == ActorSession && ev.Path == workflow.File) {
			return Result{}
		}
	case workflow.PreCommand, workflow.PreCommit:
		if onlyCheck(ev.Command) {
			return Result{}
		}
	default:
		return Result{Warnings: []string{"el workflow no es válido: " + problems}}
	}
	en.log(ev, "invalid", 0, "block", problems)
	return Result{Block: true, Reasons: []string{"polix: el workflow no es válido; corrígelo y ejecuta polix check.\n" + problems}}
}

// onlyCheck reports a command line that only validates the workflow (`polix check`, maybe after a cd).
func onlyCheck(command string) bool {
	segs := shell.Split(command)
	for _, seg := range segs {
		if !glob.Command("polix check*", seg.Text) && !glob.Command("cd *", seg.Text) {
			return false
		}
	}
	return len(segs) > 0
}

// protect applies the rules every repository gets, whatever its workflow says: nobody edits the state of
// polix, and subagents never edit the context, the rules or the workflow, which only a person changes.
func (en *Engine) protect(ev Event) (Result, bool) {
	deny := func(reason string) (Result, bool) { return Result{Block: true, Reasons: []string{reason}}, true }
	switch ev.Name {
	case workflow.PreEdit:
		abs := ev.AbsPath
		if abs != "" && !filepath.IsAbs(abs) {
			abs = filepath.Join(en.Repo.Root, abs)
		}
		if abs != "" && within(en.Store.Dir, abs) {
			return deny("El estado de polix solo lo escribe polix.")
		}
		if ev.Actor != ActorSession && glob.AnyPath(protectedPaths, ev.Path) {
			return deny(fmt.Sprintf("El subagente %s no puede editar %s: solo lo cambia una persona.", ev.Actor, ev.Path))
		}
	case workflow.PreCommand, workflow.PreCommit:
		// Reading is fine (the log is the evidence of the process); only commands that could write are denied.
		if !writesFiles.MatchString(ev.Command) {
			break
		}
		if en.namesState(ev.Command) {
			return deny("El estado de polix solo lo escribe polix; para leerlo usa polix log.")
		}
		if ev.Actor != ActorSession && protectedRef.MatchString(ev.Command) {
			return deny(fmt.Sprintf("El subagente %s no puede modificar %s: solo lo cambia una persona.", ev.Actor,
				strings.Join(protectedPaths, ", ")))
		}
	}
	return Result{}, false
}

// protectedPaths are the files only a person changes: the context, the rules and the workflow.
var protectedPaths = []string{"AGENTS.md", "CLAUDE.md", rules.Dir + "/**", workflow.File}

// protectedRef finds a protected path in a command line.
var protectedRef = regexp.MustCompile(`(^|[\s'"=/(])(AGENTS\.md|CLAUDE\.md|` + regexp.QuoteMeta(workflow.File) + `|` +
	regexp.QuoteMeta(rules.Dir) + `(/|[\s'"]|$))`)

// stateRef finds the state folder in a command line, written relative to a git directory (`.git/polix`, or
// `.git/worktrees/<name>/polix` in a worktree). The name alone is not enough: it is also the name of the binary.
var stateRef = regexp.MustCompile(`\.git/(worktrees/[^/\s'"]+/)?` + regexp.QuoteMeta(state.DirName) + `(/|[\s'"]|$)`)

// namesState reports whether a command line names the state folder, by its absolute path or below .git.
func (en *Engine) namesState(command string) bool {
	if stateRef.MatchString(filepath.ToSlash(command)) {
		return true
	}
	for _, dir := range []string{en.Store.Dir, gitx.RealPath(en.Store.Dir)} {
		if strings.Contains(command, dir) || strings.Contains(command, filepath.ToSlash(dir)) {
			return true
		}
	}
	return false
}

// writesFiles recognises commands that can create, change or delete files: a heuristic, since a shell command
// cannot be fully analysed, aimed at accidents rather than at an agent determined to get around it. A `>`
// followed by `&` duplicates a descriptor (`2>&1`) and writes nothing.
var writesFiles = regexp.MustCompile(`(^|[\s;&|(])(rm|mv|cp|tee|truncate|dd|touch|ln|install|chmod|rsync)(\s|$)|` +
	`(sed|perl)\s+(-\S*\s+)*-i|>([^&]|$)`)

// apply runs the action of a matching entry and reports whether evaluation of the event stops there.
func (en *Engine) apply(e *workflow.Entry, ev Event, res *Result) bool {
	mode := e.EffectiveMode()
	// block applies a block in the mode of the entry and reports whether the event is actually blocked.
	block := func(reason string) bool {
		switch mode {
		case workflow.ModeAudit:
			en.log(ev, e.Action(), e.Line, "audit", reason)
			return false
		case workflow.ModeWarn:
			res.Contexts = append(res.Contexts, "Aviso de polix: "+reason)
			res.Warnings = append(res.Warnings, reason)
			en.log(ev, e.Action(), e.Line, "warn", reason)
			return false
		}
		if ev.Name == workflow.Stop || ev.Name == workflow.SubagentStop {
			// Loop guard of every block of a stop (require, deny or a failing run): after max_stop_blocks blocks
			// of this entry on this same content, let the agent stop and leave a trace.
			if n := en.Store.StopBlocks(ev.Session, ev.Name, ev.Actor, e.Line, en.Hash()); n >= en.Workflow.MaxStopBlocks {
				en.log(ev, e.Action(), e.Line, "loop_guard", reason)
				res.Warnings = append(res.Warnings, fmt.Sprintf("polix deja parar tras %d bloqueos: %s", n, reason))
				return false
			}
			sb := en.entry(ev, state.KindStopBlock, "", reason)
			sb.Hash, sb.Line = en.Hash(), e.Line
			_ = en.Store.Append(sb)
		}
		res.Block = true
		res.Reasons = append(res.Reasons, reason)
		en.log(ev, e.Action(), e.Line, "block", reason)
		return true
	}
	switch e.Action() {
	case "deny":
		// Only a block that happened stops the evaluation; a warning, an audit or the loop guard let it go on.
		return block(en.expand(*e.Deny, ev))
	case "allow":
		en.log(ev, "allow", e.Line, "allow", "")
		return true
	case "require":
		block(en.expand(*e.Require, ev))
		return false
	case "run":
		// The fact is about the content the run saw: pin it before running, and record nothing if the run (or
		// anyone else meanwhile) changed it. Only a run that passed records, whatever the mode.
		var before string
		if e.Run.Record != "" {
			before = en.Hash()
		}
		if reason := en.run(e.Run, ev); reason != "" {
			block(reason)
		} else if e.Run.Record != "" {
			fact := en.expand(e.Run.Record, ev)
			en.hash = nil
			if en.Hash() != before {
				reason := fmt.Sprintf("el contenido cambió durante el run; no se registra %s", fact)
				res.Warnings = append(res.Warnings, reason)
				en.log(ev, "run", e.Line, "warn", reason)
			} else {
				en.recordFact(ev, fact)
			}
		}
		return false
	case "context":
		if text := en.context(e.Context, ev); text != "" {
			res.Contexts = append(res.Contexts, text)
		}
		return false
	case "record":
		fact := en.expand(*e.Record, ev)
		en.recordFact(ev, fact)
		return false
	}
	return false
}

// run executes a command or the checks of the rules and returns why it failed, or "".
func (en *Engine) run(r *workflow.Run, ev Event) string {
	ctx, cancel := context.WithDeadline(context.Background(), en.deadline)
	defer cancel()
	if r.Rules != "" {
		all := en.Rules()
		if en.rulesErr != nil {
			return fmt.Sprintf("No se pueden cargar las rules: %v", en.rulesErr)
		}
		var paths []string
		if r.Rules == "changed" {
			paths = en.Changed()
		} else {
			paths = []string{en.expand(r.Rules, ev)}
		}
		failures := rules.RunChecks(ctx, en.Repo.Root, rules.Matching(all, paths))
		if len(failures) == 0 {
			return ""
		}
		var b strings.Builder
		b.WriteString("Rules que no se cumplen:")
		for _, f := range failures {
			fmt.Fprintf(&b, "\n- %s (%s): %s\n%s", f.Rule.ID, f.Rule.Check, f.Rule.Description, indent(f.Output))
		}
		return b.String()
	}
	args := make([]string, len(r.Command))
	for i, a := range r.Command {
		args[i] = en.expand(a, ev)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = en.Repo.Root
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Falla %s: %v\n%s", strings.Join(args, " "), err, indent(lastLines(out.String(), 15)))
	}
	return ""
}

// context builds the text of a context action; rules are injected once per session.
func (en *Engine) context(c *workflow.Context, ev Event) string {
	switch {
	case c.File != "":
		data, err := os.ReadFile(filepath.Join(en.Repo.Root, filepath.FromSlash(en.expand(c.File, ev))))
		if err != nil {
			return ""
		}
		return string(data)
	case c.Text != "":
		return en.expand(c.Text, ev)
	case c.Rules != "":
		target := en.expand(c.Rules, ev)
		var lines []string
		for _, r := range rules.Matching(en.Rules(), []string{target}) {
			if en.Store.Injected(r.ID, ev.Session) {
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s: %s", r.ID, r.Description))
			_ = en.Store.Append(state.Entry{Kind: state.KindInjected, Agent: ev.Agent, Session: ev.Session, Rule: r.ID})
		}
		if len(lines) == 0 {
			return ""
		}
		return fmt.Sprintf("Rules que aplican a %s (ai/rules/):\n%s", target, strings.Join(lines, "\n"))
	}
	return ""
}

// matches reports whether every condition of c holds for the event.
func (en *Engine) matches(c workflow.Cond, ev Event) bool {
	for k, v := range c {
		if !en.condition(k, v, c, ev) {
			return false
		}
	}
	return true
}

// condition evaluates one condition.
func (en *Engine) condition(k string, v any, all workflow.Cond, ev Event) bool {
	list := func() []string {
		s, _ := workflow.Strings(v)
		for i := range s {
			s[i] = en.expand(s[i], ev)
		}
		return s
	}
	switch k {
	case "not":
		nested, _ := workflow.Nested(v)
		return !en.matches(nested, ev)
	case "branch":
		s, _ := v.(string)
		re, err := regexp.Compile(s)
		return err == nil && re.MatchString(en.Branch())
	case "var":
		for _, name := range list() {
			if en.Vars()[name] == "" {
				return false
			}
		}
		return true
	case "path":
		return ev.Path != "" && glob.AnyPath(list(), ev.Path)
	case "changed":
		patterns := list()
		for _, f := range en.Changed() {
			if glob.AnyPath(patterns, f) {
				return true
			}
		}
		return false
	case "file":
		return en.fileExists(list(), all["contains"], ev)
	case "contains":
		return true // evaluated together with file
	case "command":
		return len(en.matchingCommands(list(), ev)) > 0
	case "exit":
		n, _ := v.(int)
		if patterns, ok := all["command"]; ok {
			// The exit code of the line only speaks for a command whose failure propagates to it: in
			// `just test | tail`, tail decides it, so a fact like tested must not be recorded from it.
			s, _ := workflow.Strings(patterns)
			for i := range s {
				s[i] = en.expand(s[i], ev)
			}
			trusted := false
			for _, seg := range en.matchingCommands(s, ev) {
				trusted = trusted || seg.Propagates
			}
			if !trusted {
				return false
			}
		}
		return ev.ExitCode == n
	case "actor":
		for _, a := range list() {
			if a == ev.Actor {
				return true
			}
		}
		return false
	case "done":
		for _, fact := range list() {
			if !en.Store.Done(fact, en.Branch(), en.Hash()) {
				return false
			}
		}
		return true
	}
	return false
}

// matchingCommands returns the commands of the line that match one of the patterns.
func (en *Engine) matchingCommands(patterns []string, ev Event) []shell.Segment {
	var out []shell.Segment
	for _, seg := range shell.Split(ev.Command) {
		for _, p := range patterns {
			if glob.Command(p, seg.Text) {
				out = append(out, seg)
				break
			}
		}
	}
	return out
}

// fileExists reports whether a file of the repository matches one of the patterns and, with contains, holds
// that text.
func (en *Engine) fileExists(patterns []string, contains any, ev Event) bool {
	text := ""
	if s, ok := contains.(string); ok {
		text = en.expand(s, ev)
	}
	for _, f := range en.Files() {
		if !glob.AnyPath(patterns, f) {
			continue
		}
		if text == "" {
			return true
		}
		if data, err := os.ReadFile(filepath.Join(en.Repo.Root, filepath.FromSlash(f))); err == nil && bytes.Contains(data, []byte(text)) {
			return true
		}
	}
	return false
}

// expand replaces {var}, {path} and {command} in s.
func (en *Engine) expand(s string, ev Event) string {
	if !strings.Contains(s, "{") {
		return s
	}
	repl := []string{"{path}", ev.Path, "{command}", ev.Command, "{branch}", en.Branch()}
	for k, v := range en.Vars() {
		repl = append(repl, "{"+k+"}", v)
	}
	return strings.NewReplacer(repl...).Replace(s)
}

// Branch returns the current branch.
func (en *Engine) Branch() string {
	if en.branch == nil {
		b := en.Repo.Branch()
		en.branch = &b
	}
	return *en.branch
}

// Hash returns the content hash the facts are tied to.
func (en *Engine) Hash() string {
	if en.hash == nil {
		h, err := en.Repo.ContentHash(en.Store.Dir)
		if err != nil {
			h = "unknown"
		}
		en.hash = &h
	}
	return *en.hash
}

// Changed returns the files changed by the branch.
func (en *Engine) Changed() []string {
	if !en.gotChange {
		en.changed, _ = en.Repo.Changed(en.Workflow.BaseBranch())
		en.gotChange = true
	}
	return en.changed
}

// Files returns the files of the working tree that git does not ignore.
func (en *Engine) Files() []string {
	if en.files == nil {
		en.files, _ = en.Repo.Files()
	}
	return en.files
}

// Rules returns the rules of the repository.
func (en *Engine) Rules() []rules.Rule {
	if !en.gotRules {
		en.rules, en.rulesErr = rules.Load(en.Repo.Root)
		en.gotRules = true
	}
	return en.rules
}

// Vars returns the variables of the workflow; one whose expression does not match is "".
func (en *Engine) Vars() map[string]string {
	if en.vars != nil {
		return en.vars
	}
	en.vars = map[string]string{}
	if en.Workflow == nil {
		return en.vars
	}
	for name, v := range en.Workflow.Vars {
		re, err := regexp.Compile(v.Branch)
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatch(en.Branch()); m != nil {
			en.vars[name] = m[0]
			if len(m) > 1 {
				en.vars[name] = m[1]
			}
		}
	}
	return en.vars
}

func (en *Engine) recordFact(ev Event, fact string) {
	e := en.entry(ev, state.KindFact, "", "")
	e.Fact, e.Hash = fact, en.Hash()
	_ = en.Store.Append(e)
}

func (en *Engine) log(ev Event, action string, line int, outcome, reason string) {
	e := en.entry(ev, state.KindDecision, outcome, reason)
	e.Action, e.Line = action, line
	_ = en.Store.Append(e)
}

// entry fills the common fields of a log entry. The content hash costs a git call over the whole tree, so it is
// only included when this call already computed it; facts and stop blocks, which depend on it, set it.
func (en *Engine) entry(ev Event, kind, outcome, reason string) state.Entry {
	e := state.Entry{Kind: kind, Agent: ev.Agent, Session: ev.Session, Event: ev.Name, Actor: ev.Actor,
		Branch: en.Branch(), Outcome: outcome, Reason: reason}
	if en.hash != nil {
		e.Hash = *en.hash
	}
	return e
}

// within reports whether path is dir or inside it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(gitx.RealPath(dir), gitx.RealPath(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func indent(s string) string {
	if s == "" {
		return ""
	}
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ")
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
