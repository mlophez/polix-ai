package agentconf

import (
	"bytes"
	"encoding/json"
)

// settingsJSON encodes a settings or hooks file indented with two spaces, as Claude Code and Codex write theirs,
// except that each group of hooks of an event is written inline on one line, so a file with many hooks stays
// short and a change of the hooks is one line of the diff.
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
	if inlineObject(b, raw) || inlineArray(b, raw) {
		return
	}
	if err := json.Compact(b, raw); err != nil {
		b.Write(raw)
	}
}

// inlineObject writes raw inline when it is a JSON object, and reports whether it was one.
func inlineObject(b *bytes.Buffer, raw json.RawMessage) bool {
	if len(raw) == 0 || raw[0] != '{' {
		return false
	}
	members, err := decodeObject(raw)
	if err != nil {
		return false
	}
	if len(members) == 0 {
		b.WriteString("{}")
		return true
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
	return true
}

// inlineArray writes raw inline when it is a JSON array, and reports whether it was one.
func inlineArray(b *bytes.Buffer, raw json.RawMessage) bool {
	var items []json.RawMessage
	if len(raw) == 0 || raw[0] != '[' || json.Unmarshal(raw, &items) != nil {
		return false
	}
	b.WriteString("[")
	for i, it := range items {
		if i > 0 {
			b.WriteString(", ")
		}
		inlineTo(b, it)
	}
	b.WriteString("]")
	return true
}

// indentTo writes a JSON value indented with two spaces, nested at prefix.
func indentTo(b *bytes.Buffer, raw json.RawMessage, prefix string) {
	if err := json.Indent(b, raw, prefix, "  "); err != nil {
		b.Write(raw)
	}
}
