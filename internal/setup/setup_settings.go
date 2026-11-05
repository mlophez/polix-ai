package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// member is a key of a JSON object with its value as written, so a file is rewritten in its own order.
type member struct {
	key string
	val json.RawMessage
}

// RegisterHooks returns the content of a hooks file (data, which existed or not) with the hooks of polix for agent
// in place of the ones it had, and what that does to the file: "" when nothing changes, else created or updated.
// Every other key, event and group of the file is kept in its order, and the groups of polix take the place of the
// ones they replace, so a file committed by a team only changes where polix changed.
func RegisterHooks(data []byte, existed bool, agent string) ([]byte, string, error) {
	return mergeHooks(data, existed, hooks(agent))
}

// UnregisterHooks returns the content of a hooks file without the hooks of polix, and what that does to the file:
// "" when nothing changes, removed when polix was all it held, else updated.
func UnregisterHooks(data []byte, existed bool) ([]byte, string, error) {
	return mergeHooks(data, existed, nil)
}

// mergeHooks replaces the hooks of polix in the content of a hooks file with want (nil removes them). The errors
// name the part of the file that is broken; the caller adds the file.
func mergeHooks(old []byte, existed bool, want map[string][]hookGroup) ([]byte, string, error) {
	if !existed && want == nil {
		return nil, "", nil
	}
	top, err := decodeObject(old)
	if err != nil {
		return nil, "", err
	}
	hi := -1
	var events []member
	for i, m := range top {
		if m.key == "hooks" {
			hi = i
			if events, err = decodeObject(m.val); err != nil {
				return nil, "", fmt.Errorf("hooks: %w", err)
			}
		}
	}

	ours := func(ev string) []json.RawMessage {
		var out []json.RawMessage
		for _, g := range want[ev] {
			out = append(out, marshal(g))
		}
		return out
	}
	done := map[string]bool{}
	var merged []member
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.val, &groups); err != nil {
			return nil, "", fmt.Errorf("hooks.%s: %w", ev.key, err)
		}
		var kept []json.RawMessage
		for _, g := range groups {
			rest, polix := withoutPolix(g)
			if !polix {
				kept = append(kept, g)
				continue
			}
			if rest != nil {
				kept = append(kept, rest) // the other commands of a mixed group stay
			}
			if !done[ev.key] {
				kept, done[ev.key] = append(kept, ours(ev.key)...), true
			}
		}
		if !done[ev.key] {
			kept, done[ev.key] = append(kept, ours(ev.key)...), true
		}
		if len(kept) == 0 && len(groups) > 0 {
			continue // only polix was there
		}
		merged = append(merged, member{ev.key, encodeArray(kept)})
	}
	for _, ev := range hookEvents {
		if !done[ev] && len(want[ev]) > 0 {
			merged = append(merged, member{ev, encodeArray(ours(ev))})
		}
	}

	next := slices.Clone(top)
	switch {
	case hi >= 0 && len(merged) == 0 && len(events) > 0:
		next = slices.Delete(next, hi, hi+1)
	case hi >= 0:
		next[hi].val = encodeObject(merged)
	case len(merged) > 0:
		next = append(next, member{"hooks", encodeObject(merged)})
	}
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

// withoutPolix removes the handlers of polix from a group of hooks. It reports whether the group had any, and
// returns the group with the other handlers, every other key kept, or nil when polix was all it held.
func withoutPolix(raw json.RawMessage) (json.RawMessage, bool) {
	g, err := decodeObject(raw)
	if err != nil {
		return raw, false
	}
	for i, m := range g {
		if m.key != "hooks" {
			continue
		}
		var handlers []json.RawMessage
		if json.Unmarshal(m.val, &handlers) != nil {
			return raw, false
		}
		var others []json.RawMessage
		for _, h := range handlers {
			var c struct {
				Command string `json:"command"`
			}
			if json.Unmarshal(h, &c) == nil && IsPolixCommand(c.Command) {
				continue
			}
			others = append(others, h)
		}
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
	return raw, false
}

// decodeObject reads a JSON object keeping the order of its keys; empty data is an empty object.
func decodeObject(data []byte) ([]member, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil, errors.New("not a JSON object")
	}
	var out []member
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, member{t.(string), v})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("data after the JSON object")
	}
	return out, nil
}

func encodeObject(ms []member) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range ms {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(marshal(m.key))
		b.WriteByte(':')
		b.Write(m.val)
	}
	b.WriteByte('}')
	return b.Bytes()
}

func encodeArray(items []json.RawMessage) json.RawMessage {
	b := []byte{'['}
	for i, it := range items {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, it...)
	}
	return append(b, ']')
}

// marshal encodes v without escaping <, > and &, which a shell command may hold.
func marshal(v any) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return bytes.TrimSpace(b.Bytes())
}

// settingsJSON encodes a settings or hooks file indented with two spaces, as Claude Code and Codex write theirs,
// except that each group of hooks of an event is written inline on one line, so a file with many hooks stays
// short and a change of polix is one line of the diff.
func settingsJSON(top []member) []byte {
	var b bytes.Buffer
	b.WriteString("{")
	for i, m := range top {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n  ")
		b.Write(marshal(m.key))
		b.WriteString(": ")
		if events, err := decodeObject(m.val); m.key == "hooks" && err == nil && len(events) > 0 {
			writeHooks(&b, events)
			continue
		}
		indentTo(&b, m.val, "  ")
	}
	if len(top) > 0 {
		b.WriteByte('\n')
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// writeHooks writes the object hooks: one event per line, and each of its groups inline on a line of its own.
func writeHooks(b *bytes.Buffer, events []member) {
	b.WriteString("{")
	for i, ev := range events {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("\n    ")
		b.Write(marshal(ev.key))
		b.WriteString(": ")
		var groups []json.RawMessage
		if json.Unmarshal(ev.val, &groups) != nil || len(groups) == 0 {
			indentTo(b, ev.val, "    ")
			continue
		}
		b.WriteString("[")
		for j, g := range groups {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("\n      ")
			inlineTo(b, g)
		}
		b.WriteString("\n    ]")
	}
	b.WriteString("\n  }")
}

// inlineTo writes a JSON value on one line, readable: `{ "key": value, ... }` with a space after `:` and `,` and
// inside the braces, and `[a, b]` for arrays. Keys keep their order.
func inlineTo(b *bytes.Buffer, raw json.RawMessage) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '{' {
		if members, err := decodeObject(raw); err == nil {
			if len(members) == 0 {
				b.WriteString("{}")
				return
			}
			b.WriteString("{ ")
			for i, m := range members {
				if i > 0 {
					b.WriteString(", ")
				}
				b.Write(marshal(m.key))
				b.WriteString(": ")
				inlineTo(b, m.val)
			}
			b.WriteString(" }")
			return
		}
	}
	var items []json.RawMessage
	if len(raw) > 0 && raw[0] == '[' && json.Unmarshal(raw, &items) == nil {
		b.WriteString("[")
		for i, it := range items {
			if i > 0 {
				b.WriteString(", ")
			}
			inlineTo(b, it)
		}
		b.WriteString("]")
		return
	}
	if err := json.Compact(b, raw); err != nil {
		b.Write(raw)
	}
}

// indentTo writes a JSON value indented with two spaces, nested at prefix.
func indentTo(b *bytes.Buffer, raw json.RawMessage, prefix string) {
	if err := json.Indent(b, raw, prefix, "  "); err != nil {
		b.Write(raw)
	}
}

// pretty indents JSON with two spaces, as Claude Code and Codex write their files.
func pretty(raw []byte) []byte {
	var b bytes.Buffer
	_ = json.Indent(&b, raw, "", "  ")
	b.WriteByte('\n')
	return b.Bytes()
}
