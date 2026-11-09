package agentconf

import (
	"encoding/json"
	"strings"
	"testing"
)

// ours are the commands of the caller in these tests, as polix recognizes its own.
func ours(command string) bool { return strings.HasPrefix(strings.TrimSpace(command), "polix ") }

// events are the events a hooks file gets, in the order they are added.
var events = []string{"SessionStart", "PreToolUse", "Stop"}

// want are the hooks the caller registers in these tests.
var want = map[string][]Group{
	"SessionStart": {{Handlers: []Handler{{Command: "polix hook claude", Timeout: 900}}},
		{Matcher: "startup", Handlers: []Handler{{Command: "polix update --hook", Timeout: 180}}}},
	"PreToolUse": {{Matcher: "Edit|Bash", Handlers: []Handler{{Command: "polix hook claude", Timeout: 900}}}},
	"Stop":       {{Handlers: []Handler{{Command: "polix hook claude", Timeout: 900}}}},
}

// The hooks written in an empty file, each group on its line and the events in their order.
func TestMergeHooksCreates(t *testing.T) {
	got, action, err := MergeHooks(nil, false, want, events, ours)
	if err != nil || action != "created" {
		t.Fatal(action, err)
	}
	wantText := `{
  "hooks": {
    "SessionStart": [
      { "hooks": [{ "type": "command", "command": "polix hook claude", "timeout": 900 }] },
      { "matcher": "startup", "hooks": [{ "type": "command", "command": "polix update --hook", "timeout": 180 }] }
    ],
    "PreToolUse": [
      { "matcher": "Edit|Bash", "hooks": [{ "type": "command", "command": "polix hook claude", "timeout": 900 }] }
    ],
    "Stop": [
      { "hooks": [{ "type": "command", "command": "polix hook claude", "timeout": 900 }] }
    ]
  }
}
`
	if string(got) != wantText {
		t.Fatalf("got\n%s", got)
	}
}

// The rest of the file keeps its order, the hooks of the user stay and the one of the caller is replaced in place;
// a second registration changes nothing; unregistering leaves the rest.
func TestMergeHooksKeepsTheFile(t *testing.T) {
	mine := `{
  "permissions": {"allow": ["Bash(go test:*)"]},
  "hooks": {
    "Stop": [
      {"hooks": [{"type": "command", "command": "notify-send done"}]},
      {"hooks": [{"type": "command", "command": "polix hook claude"}]}
    ]
  },
  "model": "opus"
}`
	got, action, err := MergeHooks([]byte(mine), true, want, events, ours)
	if err != nil || action != "updated" {
		t.Fatal(action, err)
	}
	s := string(got)
	if p, h, m := strings.Index(s, `"permissions"`), strings.Index(s, `"hooks"`), strings.Index(s, `"model"`); p >= h || h >= m {
		t.Fatalf("the order of the keys is kept:\n%s", s)
	}
	h, _ := ReadHooks(got)
	if len(h["Stop"]) != 2 || h["Stop"][0].Handlers[0].Command != "notify-send done" ||
		h["Stop"][1].Handlers[0].Command != "polix hook claude" || h["Stop"][1].Handlers[0].Timeout != 900 {
		t.Fatalf("the hooks of the user stay and the caller is replaced in place: %+v", h["Stop"])
	}
	if len(h) != len(want) || h["PreToolUse"][0].Matcher != "Edit|Bash" {
		t.Fatalf("every event of the caller: %+v", h)
	}
	if _, action, _ := MergeHooks(got, true, want, events, ours); action != "" {
		t.Fatalf("a second registration changes nothing: %q", action)
	}
	rest, action, err := MergeHooks(got, true, nil, events, ours)
	var doc map[string]any
	if err != nil || action != "updated" || json.Unmarshal(rest, &doc) != nil || doc["model"] != "opus" || Registers(rest, ours) {
		t.Fatalf("unregister leaves the rest: %q %v\n%s", action, err, rest)
	}
	only, _, _ := MergeHooks(nil, false, want, events, ours)
	if data, action, _ := MergeHooks(only, true, nil, events, ours); action != "removed" || data != nil {
		t.Fatalf("a file of the caller alone is removed: %q", action)
	}
	if _, action, _ := MergeHooks(nil, false, nil, events, ours); action != "" {
		t.Fatalf("no file, nothing to do: %q", action)
	}
}

// A command of the caller of an earlier build is replaced in place, not kept next to the new one.
func TestMergeHooksReplacesAnOldCommand(t *testing.T) {
	got, _, err := MergeHooks([]byte(`{"hooks": {"SessionStart": [
  {"hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "polix update --hook; polix doctor --hook", "timeout": 180}]}
]}}`), true, want, events, ours)
	if err != nil || strings.Contains(string(got), "polix doctor --hook") || strings.Count(string(got), `"polix update --hook"`) != 1 {
		t.Fatalf("the old command is replaced: %v\n%s", err, got)
	}
}

// A group that mixes the caller with another command keeps the other command, and the caller ends once on the
// event.
func TestMergeHooksMixedGroup(t *testing.T) {
	mixed := `{"hooks": {"Stop": [{"hooks": [
  {"type": "command", "command": "polix hook claude"},
  {"type": "command", "command": "notify-send done"}
]}]}}`
	got, _, err := MergeHooks([]byte(mixed), true, want, events, ours)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := ReadHooks(got)
	mine, other := 0, 0
	for _, g := range h["Stop"] {
		for _, c := range g.Handlers {
			switch c.Command {
			case "polix hook claude":
				mine++
			case "notify-send done":
				other++
			}
		}
	}
	if mine != 1 || other != 1 {
		t.Fatalf("one of the caller and the other command: %+v", h["Stop"])
	}
	rest, _, _ := MergeHooks(got, true, nil, events, ours)
	h, _ = ReadHooks(rest)
	if Registers(rest, ours) || len(h["Stop"]) != 1 || h["Stop"][0].Handlers[0].Command != "notify-send done" {
		t.Fatalf("unregister leaves the other command: %+v", h)
	}
}

// Every group of hooks is written on one line.
func TestHooksGroupsInline(t *testing.T) {
	got, _, _ := MergeHooks([]byte(`{"hooks": {"Stop": [{"matcher": "", "hooks": [{"type": "prompt", "prompt": "p", "x": [1, {"a": {}}]}]}]}}`),
		true, want, events, ours)
	for l := range strings.SplitSeq(string(got), "\n") {
		inline := strings.HasPrefix(l, "      { ") && strings.Contains(l, `"hooks": [{ "type": `)
		if strings.Contains(l, `"type"`) && !inline {
			t.Fatalf("a group spans lines:\n%s", got)
		}
	}
	if !strings.Contains(string(got), `{ "matcher": "", "hooks": [{ "type": "prompt", "prompt": "p", "x": [1, { "a": {} }] }] }`) {
		t.Fatalf("a group of the user inline:\n%s", got)
	}
}

// A broken file is an error that names the part that is broken.
func TestMergeHooksBrokenFile(t *testing.T) {
	for data, prefix := range map[string]string{"[]": "not a JSON object", `{"hooks": 1}`: "hooks: not a JSON object",
		`{"hooks": {"Stop": 1}}`: "hooks.Stop: ", `{} x`: "data after the JSON object"} {
		if _, _, err := MergeHooks([]byte(data), true, want, events, ours); err == nil || !strings.HasPrefix(err.Error(), prefix) {
			t.Errorf("%s: %v", data, err)
		}
	}
}

func TestParseHooks(t *testing.T) {
	data := []byte(`{"permissions": {}, "hooks": {
		"PreToolUse": [{"matcher": "Edit|Bash", "hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]}],
		"Stop": [{"hooks": [{"type": "command", "command": "echo hi"}, {"type": "prompt", "prompt": "p"}]}]}}`)
	got, err := ParseHooks(data)
	if err != nil {
		t.Fatal(err)
	}
	pre, stop := got["PreToolUse"], got["Stop"]
	if len(got) != 2 || len(pre) != 1 || pre[0].Matcher != "Edit|Bash" ||
		len(pre[0].Handlers) != 1 || pre[0].Handlers[0] != (Handler{Command: "polix hook claude", Timeout: 900}) {
		t.Fatalf("%+v", got)
	}
	if len(stop) != 1 || len(stop[0].Handlers) != 2 || stop[0].Handlers[1].Command != "" {
		t.Fatalf("a handler of another type has no command: %+v", stop)
	}
	if got, err := ParseHooks([]byte(`{"permissions": {"allow": []}}`)); err != nil || len(got) != 0 {
		t.Fatalf("no hooks: %+v, %v", got, err)
	}
	for _, bad := range []string{"{", "", `{"hooks": []}`} {
		if _, err := ParseHooks([]byte(bad)); err == nil {
			t.Errorf("%q is not a settings file", bad)
		}
	}
}

func TestReadHooksAndRegisters(t *testing.T) {
	data, _, _ := MergeHooks(nil, false, want, events, ours)
	if !Registers(data, ours) || Registers([]byte(`{"hooks": {"Stop": [{"hooks": [{"command": "x"}]}]}}`), ours) ||
		Registers(nil, ours) || Registers([]byte("{"), ours) {
		t.Fatal("registers")
	}
	if h, err := ReadHooks([]byte(" \n")); err != nil || h != nil {
		t.Fatal("an empty file has no hooks")
	}
	if _, err := ReadHooks([]byte("{")); err == nil {
		t.Fatal("a file that is not JSON")
	}
}
