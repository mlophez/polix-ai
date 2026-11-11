package load

import (
	"context"
	"encoding/json"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/jsonl"
)

// Adapter implements Port on the disk. The entry passes it as the port; there is no constructor.
type Adapter struct{}

// entryJSON is the shape of one line of the log, with the fields of state.Entry in its order, so it converts to
// one; AnswerHook, the only writer of the log, encodes the same shape (logLine of internal/app/hook/answer).
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

// ReadLog reads the log at path through internal/platform/jsonl, skipping and counting the lines over
// state.MaxLine, and decodes each line with entryJSON: every line that decodes is kept as it is, even without kind,
// and the lines that do not decode are left out. A log that does not exist has no lines; one that cannot be read is
// the error.
func (a Adapter) ReadLog(_ context.Context, path string) ([]state.Entry, int, error) {
	lines, skipped, err := jsonl.Read(path, state.MaxLine)
	if err != nil {
		return nil, 0, err
	}
	var entries []state.Entry
	for _, line := range lines {
		var j entryJSON
		if json.Unmarshal(line, &j) == nil {
			entries = append(entries, state.Entry(j))
		}
	}
	return entries, skipped, nil
}
