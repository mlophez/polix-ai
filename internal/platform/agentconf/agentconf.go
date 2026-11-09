// Package agentconf reads and writes the configuration of the agents Claude Code and Codex as their files and
// CLIs hold it: the hooks of a settings file of Claude Code or a hooks file of Codex (JSON), rewritten in place
// with every other setting, hook and key kept in its order; the table of a plugin in the configuration of Codex
// (TOML); the manifests of a local marketplace of plugins; and what `claude ... --json` prints. It knows the
// formats, not polix: what is a hook of polix and which ones to write come from the caller.
package agentconf

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// member is a key of a JSON object with its value as written, so a file is rewritten in its own order.
type member struct {
	key string
	val json.RawMessage
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

// encodeObject encodes the members of an object compactly, in their order.
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

// encodeArray encodes items as a compact JSON array.
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

// pretty indents JSON with two spaces, as Claude Code and Codex write their files.
func pretty(raw []byte) []byte {
	var b bytes.Buffer
	_ = json.Indent(&b, raw, "", "  ")
	b.WriteByte('\n')
	return b.Bytes()
}
