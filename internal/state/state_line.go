package state

import (
	"encoding/json"

	"github.com/mlophez/polix-ai/pkg/text"
)

// entryJSON is the shape of one line of log.jsonl.
type entryJSON struct {
	At      string `json:"at"`
	Kind    string `json:"kind"`
	Agent   string `json:"agent,omitempty"`
	Session string `json:"session,omitempty"`
	Event   string `json:"event,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Hash    string `json:"hash,omitempty"`
	Fact    string `json:"fact,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Action  string `json:"action,omitempty"`
	Line    int    `json:"line,omitempty"`
	Outcome string `json:"outcome,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// MaxLine is the longest line of the log that is read; longer lines are skipped.
const MaxLine = 4 * 1024 * 1024

// MaxReason is the longest reason stored in an entry; longer reasons are cut, so every line stays far below
// MaxLine.
const MaxReason = 8 * 1024

// Parse builds the state from the lines of the log and the count of lines skipped for being over MaxLine, as
// pkg/jsonl reads them; every line that decodes is kept as it is, and only the lines that do not decode are
// left out.
func Parse(lines [][]byte, skipped int) (State, error) {
	var entries []Entry
	for _, line := range lines {
		var j entryJSON
		if json.Unmarshal(line, &j) == nil {
			entries = append(entries, Entry(j))
		}
	}
	s := State{Entries: entries, Skipped: skipped}
	return s, s.Valid()
}

// EncodeLine encodes e as one line of the log, without the newline, its reason cut to MaxReason.
func EncodeLine(e Entry) ([]byte, error) {
	if err := e.Valid(); err != nil {
		return nil, err
	}
	j := entryJSON(e)
	j.Reason = text.Cut(j.Reason, MaxReason)
	return json.Marshal(j)
}
