// Package adapters reads the log of polix: Log loads it (state.Load), one file of a session at a time, so cmd/polix
// builds it once and passes its method Load to every use case that reads the log. It reads and decodes; what the
// records mean is decided by state.State.Loaded.
package adapters

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/jsonl"
)

// Log loads the log of a session. Its zero value works. It keeps nothing: the log is appended to by the process that
// reads it, so each Load reads it again.
type Log struct{}

// lineJSON is the shape of one line of the log, whatever its kind: the keys of an event line (at, agent, session,
// event, actor, agent_id, branch, head, hash, input, actions and observed, in the order AnswerHook writes them,
// internal/app/hook/features/answer) and the flat keys of a legacy line, which the key kind tells apart.
type lineJSON struct {
	At       string            `json:"at"`
	Kind     *string           `json:"kind"`
	Agent    string            `json:"agent"`
	Session  string            `json:"session"`
	Event    string            `json:"event"`
	Actor    string            `json:"actor"`
	AgentID  string            `json:"agent_id"`
	Branch   string            `json:"branch"`
	Head     string            `json:"head"`
	Hash     string            `json:"hash"`
	Input    inputJSON         `json:"input"`
	Actions  []actionJSON      `json:"actions"`
	Observed []observationJSON `json:"observed"`
	// The flat keys of a legacy line.
	Before  string `json:"before"`
	Refs    string `json:"refs"`
	Rule    string `json:"rule"`
	Fact    string `json:"fact"`
	Action  string `json:"action"`
	Line    int    `json:"line"`
	Key     string `json:"key"`
	Outcome string `json:"outcome"`
	Reason  string `json:"reason"`
}

// inputJSON is the input of an event line.
type inputJSON struct {
	Command string `json:"command"`
	ToolUse string `json:"tool_use"`
	Path    string `json:"path"`
	Skill   string `json:"skill"`
	Args    string `json:"args"`
	Prompt  string `json:"prompt"`
	Source  string `json:"source"`
}

// actionJSON is an element of the actions of an event line.
type actionJSON struct {
	Line       int    `json:"line"`
	Action     string `json:"action"`
	Outcome    string `json:"outcome"`
	Key        string `json:"key"`
	Reason     string `json:"reason"`
	DurationMS *int64 `json:"duration_ms"`
}

// observationJSON is an element of the observed list of an event line.
type observationJSON struct {
	Line    int    `json:"line"`
	Outcome string `json:"outcome"`
	Key     string `json:"key"`
	Hash    string `json:"hash"`
	Refs    string `json:"refs"`
	Before  string `json:"before"`
}

// logRead is what read finds in a log, with the fields of state.State in its order, so it converts to one.
type logRead struct {
	Records []state.Record
	Skipped int
}

// Load loads the log at path: it reads the records of its lines and has state.State.Loaded decide what they load.
// It never fails: what stops the reading is data of its result. Every call reads the log again.
func (l *Log) Load(_ context.Context, path string) state.Loaded {
	read, err := l.read(path)
	return state.State(read).Loaded(err)
}

// read reads the log at path through internal/platform/jsonl, skipping and counting the lines over state.MaxLine,
// and decodes each line with lineJSON: a line with the key kind is a legacy record, one without it an event line, each
// with the line as it was read. A line that does not decode, or decodes as neither (state.Record.Valid), is left out.
// A log that does not exist has no lines; one that cannot be read is the error.
func (l *Log) read(path string) (logRead, error) {
	lines, skipped, err := jsonl.Read(path, state.MaxLine)
	if err != nil {
		return logRead{}, err
	}
	var records []state.Record
	for _, line := range lines {
		var j lineJSON
		if !bytes.HasPrefix(bytes.TrimSpace(line), []byte("{")) || json.Unmarshal(line, &j) != nil {
			continue
		}
		r, err := j.record(string(line))
		if err != nil {
			continue
		}
		records = append(records, r)
	}
	return logRead{Records: records, Skipped: skipped}, nil
}

// record converts the line, which raw is as it was read, to a record, and tells why it is not one: an event line with
// its input, actions and observations, or a legacy record, which holds the one action or observation of its flat
// line, with the hash of the line on its observation, and its rule and fact. A kind an earlier polix no longer writes,
// or a newer one does, holds the fields it carries on its one action.
func (j lineJSON) record(raw string) (state.Record, error) {
	var legacy, rule, fact, hash string
	var input state.Input
	var actions []state.Action
	var observed []state.Observation
	switch {
	case j.Kind == nil:
		input, hash = state.Input(j.Input), j.Hash
		for _, a := range j.Actions {
			actions = append(actions, state.Action(a))
		}
		for _, o := range j.Observed {
			observed = append(observed, state.Observation(o))
		}
	case *j.Kind == state.LegacyObserved:
		legacy = *j.Kind
		observed = append(observed, state.Observation(observationJSON{Line: j.Line, Outcome: j.Outcome, Key: j.Key,
			Hash: j.Hash, Refs: j.Refs, Before: j.Before}))
	default:
		legacy, rule, fact, hash = *j.Kind, j.Rule, j.Fact, j.Hash
		actions = append(actions, state.Action(actionJSON{Line: j.Line, Action: j.Action, Outcome: j.Outcome,
			Key: j.Key, Reason: j.Reason}))
	}
	r := state.Record{At: j.At, Agent: j.Agent, Session: j.Session, Event: j.Event, Actor: j.Actor,
		AgentID: j.AgentID, Branch: j.Branch, Head: j.Head, Hash: hash, Input: input, Actions: actions,
		Observations: observed, Legacy: legacy, Rule: rule, Fact: fact, Raw: raw}
	return r, r.Valid()
}
