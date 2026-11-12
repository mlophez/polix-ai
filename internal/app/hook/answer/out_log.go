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

// logLine is the shape of one line of the file of a session that AnswerHook writes, the fields of state.Entry with
// the names of the log; the empty ones are left out. LoadState (internal/app/state/load) decodes the same shape.
type logLine struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event,omitempty"`
	Actor   string `json:"actor,omitempty"`
	AgentID string `json:"agent_id,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Before  string `json:"before,omitempty"`
	Head    string `json:"head,omitempty"`
	Refs    string `json:"refs,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Fact    string `json:"fact,omitempty"`
	Action  string `json:"action,omitempty"`
	Line    int    `json:"line,omitempty"`
	Key     string `json:"key,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

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

// Append writes e as one line at the end of the file of its session, stamped with the time when it has none; an
// entry that is not valid, one without a session among them, is not written.
func (a *Adapter) Append(_ context.Context, e state.Entry) error {
	at := e.At
	if at == "" {
		at = time.Now().Format(time.RFC3339)
	}
	stamped := state.Entry{At: at, Kind: e.Kind, Agent: e.Agent, Session: e.Session, Event: e.Event, Actor: e.Actor,
		AgentID: e.AgentID, Branch: e.Branch, Hash: e.Hash, Before: e.Before, Head: e.Head, Refs: e.Refs, Rule: e.Rule, Action: e.Action, Line: e.Line, Key: e.Key,
		Outcome: e.Outcome, Reason: e.Reason}
	if err := stamped.Valid(); err != nil {
		return err
	}
	line, err := a.encodeLine(stamped)
	if err != nil {
		return fmt.Errorf("log entry: %w", err)
	}
	p, err := a.logPath(state.Session(stamped.Session))
	if err != nil {
		return err
	}
	return jsonl.Append(p, line)
}

// logPath is the path of the file of the session in the state folder of the repository, or the error of a session
// that names no file.
func (a *Adapter) logPath(session state.Session) (string, error) {
	name, err := session.File()
	if err != nil {
		return "", err
	}
	return filepath.Join(a.Repo.GitDir, state.DirName, state.SessionsDir, name), nil
}

// encodeLine encodes e as one line of the log, without the newline, its reason cut to state.MaxReason, so no line
// comes near state.MaxLine.
func (a *Adapter) encodeLine(e state.Entry) ([]byte, error) {
	if err := e.Valid(); err != nil {
		return nil, err
	}
	l := logLine(e)
	l.Reason = text.Cut(l.Reason, state.MaxReason)
	return json.Marshal(l)
}
