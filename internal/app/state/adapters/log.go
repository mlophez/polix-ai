// Package adapters reads the log of polix: Log loads it (state.Load), one file of a session at a time, so cmd/polix
// builds it once and passes its method Load to every use case that reads the log. It reads and decodes; what the
// entries mean is decided by state.State.Loaded.
package adapters

import (
	"context"
	"encoding/json"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/jsonl"
)

// Log loads the log of a session. Its zero value works. It keeps nothing: the log is appended to by the process that
// reads it, so each Load reads it again.
type Log struct{}

// entryJSON is the shape of one line of the log, with the fields of state.Entry in its order, so it converts to
// one; AnswerHook, the only writer of the log, encodes the same shape (logLine of internal/app/hook/features/answer).
type entryJSON struct {
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

// logRead is what read finds in a log, with the fields of state.State in its order, so it converts to one.
type logRead struct {
	Entries []state.Entry
	Skipped int
}

// Load loads the log at path: it reads the entries of its lines and has state.State.Loaded decide what they load.
// It never fails: what stops the reading is data of its result. Every call reads the log again.
func (l *Log) Load(_ context.Context, path string) state.Loaded {
	read, err := l.read(path)
	return state.State(read).Loaded(err)
}

// read reads the log at path through internal/platform/jsonl, skipping and counting the lines over state.MaxLine,
// and decodes each line with entryJSON: every line that decodes is kept as it is, even without kind, and the lines
// that do not decode are left out. A log that does not exist has no lines; one that cannot be read is the error.
func (l *Log) read(path string) (logRead, error) {
	lines, skipped, err := jsonl.Read(path, state.MaxLine)
	if err != nil {
		return logRead{}, err
	}
	var entries []state.Entry
	for _, line := range lines {
		var j entryJSON
		if json.Unmarshal(line, &j) == nil {
			entries = append(entries, state.Entry(j))
		}
	}
	return logRead{Entries: entries, Skipped: skipped}, nil
}
