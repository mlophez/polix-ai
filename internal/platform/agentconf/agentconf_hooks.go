package agentconf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
)

// Group is a group of the hooks of an event in a settings file of Claude Code or a hooks file of Codex: the
// matcher of the tools it runs on (empty for every tool) and its handlers.
type Group struct {
	Matcher  string
	Handlers []Handler
}

// Handler is a command a group of hooks runs, with its timeout in seconds, 0 when it sets none, and its status,
// the text the agent shows while it runs, empty for none. A handler of another type (a prompt) has no command.
type Handler struct {
	Command string
	Timeout float64
	Status  string
}

// hookGroup and hookHandler are the JSON format of the hooks of Claude Code and Codex: by event, groups with a
// matcher, each with its handlers. They decode the hooks a file holds and write the ones the caller registers.
type hookGroup struct {
	Matcher string        `json:"matcher,omitempty"`
	Hooks   []hookHandler `json:"hooks"`
}

type hookHandler struct {
	Type    string  `json:"type"`
	Command string  `json:"command"`
	Timeout float64 `json:"timeout,omitempty"`
	Status  string  `json:"statusMessage,omitempty"`
}

// hooksFileJSON is the part of a settings file of Claude Code, or a hooks file of Codex, that holds the hooks.
type hooksFileJSON struct {
	Hooks map[string][]hookGroup `json:"hooks"`
}

// merger replaces the hooks of the caller (ours) in the events of a hooks file with want, already encoded, and
// records the events it wrote them to.
type merger struct {
	want map[string][]json.RawMessage
	ours func(command string) bool
	done map[string]bool
}

// ParseHooks returns the groups of hooks of a settings file of Claude Code, by event, or the error of the JSON
// decoder when its content is not JSON, an empty content included.
func ParseHooks(data []byte) (map[string][]Group, error) {
	var doc hooksFileJSON
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	out := map[string][]Group{}
	for ev, groups := range doc.Hooks {
		for _, g := range groups {
			group := Group{Matcher: g.Matcher}
			for _, h := range g.Hooks {
				group.Handlers = append(group.Handlers, Handler{Command: h.Command, Timeout: h.Timeout, Status: h.Status})
			}
			out[ev] = append(out[ev], group)
		}
	}
	return out, nil
}

// ReadHooks returns the groups of hooks of a hooks file, by event, as ParseHooks; an empty file has none.
func ReadHooks(data []byte) (map[string][]Group, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	return ParseHooks(data)
}

// Registers reports whether the content of a hooks file runs any command of ours; a file that is not JSON runs
// none.
func Registers(data []byte, ours func(command string) bool) bool {
	h, _ := ReadHooks(data)
	for _, groups := range h {
		for _, g := range groups {
			if slices.ContainsFunc(g.Handlers, func(c Handler) bool { return ours(c.Command) }) {
				return true
			}
		}
	}
	return false
}

// MergeHooks returns the content of a hooks file (old, which existed or not) with want in place of the hooks
// whose command is one of ours, and what that does to the file: "" when nothing changes, removed when the hooks
// of ours were all it held and want is nil, else created or updated. Every other key, event and group of the file
// is kept in its order, the groups of want take the place of the ones they replace, and the events the file does
// not have are added in the order of events, so a file committed by a team only changes where the hooks of ours
// changed. A nil want removes them. The errors name the part of the file that is broken; the caller adds the file.
func MergeHooks(old []byte, existed bool, want map[string][]Group, events []string,
	ours func(command string) bool) ([]byte, string, error) {
	if !existed && want == nil {
		return nil, "", nil
	}
	top, err := decodeObject(old)
	if err != nil {
		return nil, "", err
	}
	at, present, err := hooksOf(top)
	if err != nil {
		return nil, "", err
	}
	m := merger{want: encodeGroups(want), ours: ours, done: map[string]bool{}}
	merged, err := m.merge(present)
	if err != nil {
		return nil, "", err
	}
	next := withHooks(top, at, append(merged, m.added(events)...), len(present) > 0)
	data := settingsJSON(next)
	switch {
	case existed && bytes.Equal(data, settingsJSON(top)):
		return nil, "", nil
	case want == nil && len(next) == 0:
		return nil, "removed", nil
	case existed:
		return data, "updated", nil
	}
	return data, "created", nil
}

// hooksOf returns the index of the key hooks in the members of a file, -1 without it, and its events.
func hooksOf(top []member) (int, []member, error) {
	at := -1
	var events []member
	for i, m := range top {
		if m.key != "hooks" {
			continue
		}
		at = i
		var err error
		if events, err = decodeObject(m.val); err != nil {
			return 0, nil, fmt.Errorf("hooks: %w", err)
		}
	}
	return at, events, nil
}

// withHooks returns the members of a file with the events merged as its hooks, at the index at of the key hooks
// (-1 without it): the key goes when the file had events and none is left, and comes when there was none.
func withHooks(top []member, at int, merged []member, had bool) []member {
	next := slices.Clone(top)
	switch {
	case at >= 0 && len(merged) == 0 && had:
		next = slices.Delete(next, at, at+1)
	case at >= 0:
		next[at].val = encodeObject(merged)
	case len(merged) > 0:
		next = append(next, member{"hooks", encodeObject(merged)})
	}
	return next
}

// encodeGroups encodes the groups of want, by event, as handlers of type command, with their status.
func encodeGroups(want map[string][]Group) map[string][]json.RawMessage {
	out := map[string][]json.RawMessage{}
	for ev, groups := range want {
		for _, g := range groups {
			group := hookGroup{Matcher: g.Matcher}
			for _, h := range g.Handlers {
				group.Hooks = append(group.Hooks,
					hookHandler{Type: "command", Command: h.Command, Timeout: h.Timeout, Status: h.Status})
			}
			out[ev] = append(out[ev], marshal(group))
		}
	}
	return out
}

// merge returns the events of a file with the groups of want in place of the hooks of ours: the first group that
// held one of them takes the groups of want, the other commands of a mixed group stay, and an event the hooks of
// ours were all it held goes.
func (m merger) merge(events []member) ([]member, error) {
	var merged []member
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.val, &groups); err != nil {
			return nil, fmt.Errorf("hooks.%s: %w", ev.key, err)
		}
		kept := m.event(ev.key, groups)
		if len(kept) == 0 && len(groups) > 0 {
			continue // only ours was there
		}
		merged = append(merged, member{ev.key, encodeArray(kept)})
	}
	return merged, nil
}

// event returns the groups of an event with the groups of want in place of the first one that held a hook of
// ours, or after the others when none did.
func (m merger) event(ev string, groups []json.RawMessage) []json.RawMessage {
	var kept []json.RawMessage
	for _, g := range groups {
		rest, ours := m.without(g)
		if !ours {
			kept = append(kept, g)
			continue
		}
		if rest != nil {
			kept = append(kept, rest) // the other commands of a mixed group stay
		}
		kept = m.place(ev, kept)
	}
	return m.place(ev, kept)
}

// place appends the groups of want of ev to kept, once per event.
func (m merger) place(ev string, kept []json.RawMessage) []json.RawMessage {
	if m.done[ev] {
		return kept
	}
	m.done[ev] = true
	return append(kept, m.want[ev]...)
}

// added returns the events of want the file did not have, in the order of events.
func (m merger) added(events []string) []member {
	var out []member
	for _, ev := range events {
		if !m.done[ev] && len(m.want[ev]) > 0 {
			out = append(out, member{ev, encodeArray(m.want[ev])})
		}
	}
	return out
}

// without removes the handlers of ours from a group of hooks. It reports whether the group had any, and returns
// the group with the other handlers, every other key kept, or nil when ours were all it held.
func (m merger) without(raw json.RawMessage) (json.RawMessage, bool) {
	g, err := decodeObject(raw)
	if err != nil {
		return raw, false
	}
	i := slices.IndexFunc(g, func(k member) bool { return k.key == "hooks" })
	if i < 0 {
		return raw, false
	}
	var handlers []json.RawMessage
	if json.Unmarshal(g[i].val, &handlers) != nil {
		return raw, false
	}
	others := slices.DeleteFunc(slices.Clone(handlers), func(h json.RawMessage) bool {
		var c struct {
			Command string `json:"command"`
		}
		return json.Unmarshal(h, &c) == nil && m.ours(c.Command)
	})
	switch {
	case len(others) == len(handlers):
		return raw, false
	case len(others) == 0:
		return nil, true
	}
	g = slices.Clone(g)
	g[i].val = encodeArray(others)
	return encodeObject(g), true
}
