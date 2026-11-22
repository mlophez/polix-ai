package answer

import (
	"strings"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// passNotice is a notice of the verb notify that reached the answer without always: the line of its entry and its
// expanded text, written as notified once the whole call is known not to block (see settle).
type passNotice struct {
	line int
	text string
}

// notify carries out the verb notify of the entry e: it adds the expanded message to the messages for the user of ans
// (agent.Answer.Warn), never to the agent, and logs a notify decision with it. A message that expands to nothing shows
// nothing. With always, or on an event whose notices all show every time (every), it shows every time its entry
// applies; otherwise once per session and text, whatever the actor: it is skipped, and the skip logged, while the log
// has a notified line of the session with the line of the entry and that text (see notified). A placeholder that
// changes its text makes a new notice.
func (r *pass) notify(e workflow.Entry, ans agent.Answer) agent.Answer {
	n := *e.Action.Notify
	text := r.expand(n.Message)
	if r.waiting() || strings.TrimSpace(text) == "" {
		return ans
	}
	if !n.Always && !r.every {
		already := r.notified(e.Line, text)
		if r.waiting() {
			return ans
		}
		if already {
			r.log(workflow.VerbNotify, e.Line, "skip", "already shown to the user in this session")
			return ans
		}
	}
	r.log(workflow.VerbNotify, e.Line, "notify", text)
	return ans.Warn(text)
}

// notified reports whether the notice of the entry at line with text already reached the user in the session of the
// event: the log has a notified observation of the session with that line and text, of any actor, or an earlier event
// of the call showed it (its record pending, see settle, which the log of the pass holds, see currentState). Nothing
// makes it due again in the session, neither a start nor the end of a subagent. A log that cannot be read counts as
// not notified, with a warning (see warnLog), so the notice shows every time.
func (r *pass) notified(line int, text string) bool {
	st, err := r.currentState()
	if r.waiting() {
		return false
	}
	if err != nil {
		r.warnLog(err)
		return false
	}
	q := state.Query{Outcome: state.OutcomeNotified, Session: r.ev.Event.Session, Line: line, Key: text}
	if q.Valid() != nil {
		return false
	}
	_, logged := st.Latest(q)
	return logged
}

// shown returns notices with the notice the entry e added to the answer, its messages for the user added, when it is
// a notify without always: the other verbs, a notice skipped and a notice with always add none.
func shown(notices []passNotice, e workflow.Entry, added []string) []passNotice {
	if e.Action.Notify == nil || e.Action.Notify.Always {
		return notices
	}
	for _, text := range added {
		notices = append(notices, passNotice{line: e.Line, text: text})
	}
	return notices
}

// settle settles, once the event is answered with ans, the notices of this event: each is noted as a notified
// observation of the event, without hash, in the order they were shown. The events of one call give one answer, and
// neither agent shows a message of a blocked one, so a block of the event, or of an earlier one (hook.Facts.Blocked),
// notes none and they stay due; their messages are logged as not shown (see warn), and a later event that blocks drops
// the notices of the earlier ones the same way (hook.Facts.Answered). Otherwise they go to the record of the event,
// which the last event of the call appends (see flush).
func (r *pass) settle(ans agent.Answer) {
	if ans.Block || r.ev.Facts.Blocked {
		return
	}
	for _, n := range r.notices {
		_ = r.note(n.line, state.OutcomeNotified, n.text, "")
	}
}
