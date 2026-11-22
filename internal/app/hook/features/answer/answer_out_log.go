package answer

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/jsonl"
	"github.com/mlophez/polix-ai/internal/platform/text"
)

// logLine is the shape of one line of the file of a session that AnswerHook writes, the event line: the keys in the
// order of the log, the empty ones left out. stateadapters.Log (internal/app/state/adapters) decodes the same shape.
type logLine struct {
	At       string        `json:"at"`
	Agent    string        `json:"agent,omitempty"`
	Session  string        `json:"session,omitempty"`
	Event    string        `json:"event,omitempty"`
	Actor    string        `json:"actor,omitempty"`
	AgentID  string        `json:"agent_id,omitempty"`
	Branch   string        `json:"branch,omitempty"`
	Head     string        `json:"head,omitempty"`
	Hash     string        `json:"hash,omitempty"`
	Input    *logInput     `json:"input,omitempty"`
	Actions  []logAction   `json:"actions,omitempty"`
	Observed []logObserved `json:"observed,omitempty"`
}

// logInput is the input of an event line: the data of the event that applies, each text cut to maxInput.
type logInput struct {
	Command string `json:"command,omitempty"`
	ToolUse string `json:"tool_use,omitempty"`
	Path    string `json:"path,omitempty"`
	Skill   string `json:"skill,omitempty"`
	Args    string `json:"args,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
	Source  string `json:"source,omitempty"`
}

// logAction is an element of the actions of an event line. The duration is written on every run that ran, 0 included.
type logAction struct {
	Line       int    `json:"line,omitempty"`
	Action     string `json:"action"`
	Outcome    string `json:"outcome"`
	Key        string `json:"key,omitempty"`
	Reason     string `json:"reason,omitempty"`
	DurationMS *int64 `json:"duration_ms,omitempty"`
}

// logObserved is an element of the observed list of an event line.
type logObserved struct {
	Line    int    `json:"line,omitempty"`
	Outcome string `json:"outcome"`
	Key     string `json:"key,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Refs    string `json:"refs,omitempty"`
	Before  string `json:"before,omitempty"`
}

// maxInput is the longest text of the input of an event line, cut with text.Shorten: the start of a prompt or of a
// command says which one it was, and the log never keeps a heredoc or a prompt whole.
const maxInput = 200

// LoadState loads the file of the session through the loader of the log, which polix log shares: the lines over
// state.MaxLine are skipped and counted, and every line that decodes is kept. A file that does not exist is an empty
// log; one that cannot be read is an error, and so is a session without an id, which names no file.
func (a *Adapter) LoadState(ctx context.Context, session state.Session) (state.State, error) {
	p, err := a.logPath(session)
	if err != nil {
		return state.State{}, err
	}
	loaded := a.State(ctx, p)
	return loaded.State, loaded.Err
}

// Append writes r as one line at the end of the file of its session, stamped with the time when it has none; a record
// that is not to be written (see state.Record.Writable), one without a session among them, is not written.
func (a *Adapter) Append(_ context.Context, r state.Record) error {
	line, err := a.encodeLine(r.Stamped(time.Now().Format(time.RFC3339)))
	if err != nil {
		return fmt.Errorf("log record: %w", err)
	}
	p, err := a.logPath(state.Session(r.Session))
	if err != nil {
		return err
	}
	return jsonl.Append(p, line)
}

// Now returns the time of the clock as RFC 3339 with its offset, the at of the record of each event the shell
// evaluates.
func (a *Adapter) Now(context.Context) string { return time.Now().Format(time.RFC3339) }

// logPath is the path of the file of the session in the state folder of the repository, or the error of a session
// that names no file.
func (a *Adapter) logPath(session state.Session) (string, error) {
	name, err := session.File()
	if err != nil {
		return "", err
	}
	return filepath.Join(a.Repo.GitDir, state.DirName, state.SessionsDir, name), nil
}

// encodeLine encodes r as one event line of the log, without the newline: each text of its input cut to maxInput
// characters and each reason to state.MaxReason, so no line comes near state.MaxLine.
func (a *Adapter) encodeLine(r state.Record) ([]byte, error) {
	if err := r.Writable(); err != nil {
		return nil, err
	}
	l := logLine{At: r.At, Agent: r.Agent, Session: r.Session, Event: r.Event, Actor: r.Actor, AgentID: r.AgentID,
		Branch: r.Branch, Head: r.Head, Hash: r.Hash}
	if in := r.Input; in != (state.Input{}) {
		l.Input = &logInput{Command: text.Shorten(in.Command, maxInput), ToolUse: text.Shorten(in.ToolUse, maxInput),
			Path: text.Shorten(in.Path, maxInput), Skill: text.Shorten(in.Skill, maxInput),
			Args: text.Shorten(in.Args, maxInput), Prompt: text.Shorten(in.Prompt, maxInput),
			Source: text.Shorten(in.Source, maxInput)}
	}
	for _, act := range r.Actions {
		l.Actions = append(l.Actions, logAction{Line: act.Line, Action: act.Action, Outcome: act.Outcome, Key: act.Key,
			Reason: text.Cut(act.Reason, state.MaxReason), DurationMS: act.DurationMS})
	}
	for _, o := range r.Observations {
		l.Observed = append(l.Observed, logObserved(o))
	}
	return json.Marshal(l)
}
