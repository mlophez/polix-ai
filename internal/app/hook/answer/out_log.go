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

// logLine is the shape of one line of log.jsonl that AnswerHook writes, the fields of state.Entry with the names of
// the log; the empty ones are left out. LoadState (internal/app/state/load) decodes the same shape.
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

// LoadState loads the log through the loader of the log, which polix log shares: the lines over state.MaxLine are
// skipped and counted, and every line that decodes is kept. A log that does not exist is empty; one that cannot be
// read is an error.
func (a *Adapter) LoadState(ctx context.Context) (state.State, error) {
	loaded := a.State(ctx, a.logPath())
	return loaded.State, loaded.Err
}

// Append writes e as one line at the end of the log, stamped with the time when it has none.
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
	return jsonl.Append(a.logPath(), line)
}

// logPath is the path of the log of the repository.
func (a *Adapter) logPath() string {
	return filepath.Join(a.Repo.GitDir, state.DirName, state.LogName)
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
