package answer

import (
	"fmt"
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/rule"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// inject carries out the verb inject of the entry e: the text, the file or the rules that apply to a path (see
// injectRules). On the events whose injects all inject every time (start, see session), or with always, it injects
// every time it applies. Otherwise it injects once: keyed by the session, the actor, the line of the entry and the
// expanded text or file path (one key per rule id for the rules), it is skipped while not due (see due) and, once
// the answer shows it reaches the AI, an observed `injected` line is written for it (see writeInjected). A
// placeholder that changes on every event makes a new key, so it injects every time.
func (r *pass) inject(e workflow.Entry, ans agent.Answer) agent.Answer {
	in := *e.Action.Inject
	once := !in.Always && !r.always
	if in.Rules != "" {
		return r.injectRules(e.Line, in.Rules, once, ans)
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
			r.log(workflow.VerbInject, e.Line, "skip", "already injected into "+r.ev.Event.Actor+" in this session")
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
		r.injected = append(r.injected, passInject{line: e.Line, key: key})
	}
	return ans.Inform(text)
}

// injectRules injects into ans the rules that apply to the expanded target of the entry at line (see contextRules).
func (r *pass) injectRules(line int, target string, once bool, ans agent.Answer) agent.Answer {
	target = r.expand(target)
	if r.waiting() {
		return ans
	}
	if t := r.contextRules(line, target, once); t != "" && !r.waiting() {
		ans = ans.Inform(t)
	}
	return ans
}

// writeInjected writes an observed `injected` line, without hash, for each once-only inject that reached the answer,
// in the order they were injected. The family calls it only when the contexts of the answer reach the AI (see
// session).
func (r *pass) writeInjected() {
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
func (r *pass) contextRules(line int, target string, once bool) string {
	all, _ := r.rules()
	if r.waiting() {
		return ""
	}
	matching := rule.Rules(all).Matching([]string{target})
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
			r.injected = append(r.injected, passInject{line: line, key: rl.ID})
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", rl.ID, rl.Description))
	}
	if len(lines) == 0 {
		if len(matching) > 0 {
			r.log(workflow.VerbInject, line, "skip", "the rules of "+target+" were already injected into "+r.ev.Event.Actor+
				" in this session")
		}
		return ""
	}
	return fmt.Sprintf("Rules that apply to %s (%s/):\n%s", target, r.ev.Workflow.RulesPath(),
		strings.Join(lines, "\n"))
}

// due reports whether a once-only inject of the entry at line with key is due for the actor of the event: it never
// reached the actor in the session, or after it reached it a start of the session (a new or compacted
// context, which resets every actor) or, for a subagent, the start or the end of a subagent of its type in the
// session (a run with a new context) was observed. A log that cannot be read makes it due every time, with a warning.
func (r *pass) due(line int, key string) bool {
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
	started := state.Query{Outcome: state.OutcomeStarted, Session: ev.Session, Event: workflow.Start}
	spawned := state.Query{Outcome: state.OutcomeStarted, Session: ev.Session, Event: workflow.SubagentStart,
		Actor: ev.Actor}
	finished := state.Query{Outcome: state.OutcomeFinished, Session: ev.Session, Key: ev.Actor}
	if injected.Valid() != nil || started.Valid() != nil || spawned.Valid() != nil || finished.Valid() != nil {
		return true
	}
	last := st.Index(injected)
	if last < 0 {
		return true
	}
	reset := st.Index(started)
	if ev.Actor != agent.ActorSession {
		reset = max(reset, st.Index(spawned), st.Index(finished))
	}
	return reset > last
}
