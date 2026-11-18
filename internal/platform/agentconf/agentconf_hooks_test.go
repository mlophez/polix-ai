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
	"SessionStart": {{Handlers: []Handler{{Command: "polix hook claude", Timeout: 900, Status: "polix"}}},
		{Matcher: "startup", Handlers: []Handler{{Command: "polix update --hook", Timeout: 180, Status: "polix"}}}},
	"PreToolUse": {{Matcher: "Edit|Bash", Handlers: []Handler{{Command: "polix hook claude", Timeout: 900, Status: "polix"}}}},
	"Stop":       {{Handlers: []Handler{{Command: "polix hook claude", Timeout: 900, Status: "polix"}}}},
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
      { "hooks": [{ "type": "command", "command": "polix hook claude", "timeout": 900, "statusMessage": "polix" }] },
      { "matcher": "startup", "hooks": [{ "type": "command", "command": "polix update --hook", "timeout": 180, "statusMessage": "polix" }] }
    ],
    "PreToolUse": [
      { "matcher": "Edit|Bash", "hooks": [{ "type": "command", "command": "polix hook claude", "timeout": 900, "statusMessage": "polix" }] }
    ],
    "Stop": [
      { "hooks": [{ "type": "command", "command": "polix hook claude", "timeout": 900, "statusMessage": "polix" }] }
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

// A file written without the status gets it on merge, and another hook of the group keeps its own fields.
func TestMergeHooksAddsTheStatus(t *testing.T) {
	old := `{"hooks": {"Stop": [{"hooks": [
  {"type": "command", "command": "polix hook claude", "timeout": 900},
  {"type": "command", "command": "notify-send done", "timeout": 5, "statusMessage": "notifying"}
]}]}}`
	got, action, err := MergeHooks([]byte(old), true, want, events, ours)
	if err != nil || action != "updated" {
		t.Fatal(action, err)
	}
	h, _ := ReadHooks(got)
	for ev, groups := range h {
		for _, g := range groups {
			for _, c := range g.Handlers {
				if ours(c.Command) && c.Status != "polix" {
					t.Errorf("%s: %q has no status: %+v", ev, c.Command, c)
				}
				if c.Command == "notify-send done" && c != (Handler{Command: "notify-send done", Timeout: 5, Status: "notifying"}) {
					t.Errorf("the other hook keeps its fields: %+v", c)
				}
			}
		}
	}
	if !strings.Contains(string(got), `"notify-send done", "timeout": 5, "statusMessage": "notifying"`) {
		t.Fatalf("the other hook is kept as it was:\n%s", got)
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
	status, err := ParseHooks([]byte(`{"hooks": {"Stop": [{"hooks": [
		{"type": "command", "command": "polix hook claude", "timeout": 900, "statusMessage": "polix"},
		{"type": "command", "command": "echo hi"}]}]}}`))
	if err != nil || len(status["Stop"]) != 1 || status["Stop"][0].Handlers[0].Status != "polix" ||
		status["Stop"][0].Handlers[1].Status != "" {
		t.Fatalf("the status of each handler: %+v, %v", status, err)
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

// A top-level key read as a boolean: true and false set it, any other value, a missing key and a content that is not
// a JSON object set nothing.
func TestBoolKey(t *testing.T) {
	for data, want := range map[string][2]bool{
		`{"autoMemoryEnabled": true}`:                   {true, true},
		`{"model": "opus", "autoMemoryEnabled": false}`: {false, true},
		`{"autoMemoryEnabled": "false"}`:                {false, false},
		`{"autoMemoryEnabled": null}`:                   {false, false},
		`{"hooks": {"autoMemoryEnabled": true}}`:        {false, false},
		`{"model": "opus"}`:                             {false, false},
		``:                                              {false, false},
		`{`:                                             {false, false},
		`[1, 2]`:                                        {false, false},
		`{"autoMemoryEnabled": true, "autoMemoryEnabled": 1}`: {false, false},
	} {
		if value, set := BoolKey([]byte(data), "autoMemoryEnabled"); value != want[0] || set != want[1] {
			t.Errorf("%s: %v %v", data, value, set)
		}
	}
}

// A missing key is appended as the last key, one there of any value is replaced in its place, an equal one is no
// change, and every other key keeps its bytes; a file that does not exist is created.
func TestSetKeys(t *testing.T) {
	off := []Setting{{Key: "autoMemoryEnabled", Value: false}}
	created, action, err := SetKeys(nil, false, off)
	if err != nil || action != "created" || string(created) != "{\n  \"autoMemoryEnabled\": false\n}\n" {
		t.Fatalf("a new file: %q %q %v", created, action, err)
	}
	hooks, _, _ := MergeHooks(nil, false, want, events, ours)
	got, action, err := SetKeys(hooks, true, off)
	if err != nil || action != "updated" ||
		!strings.HasSuffix(string(got), "    ]\n  },\n  \"autoMemoryEnabled\": false\n}\n") ||
		!strings.HasPrefix(string(got), strings.TrimSuffix(string(hooks), "\n}\n")) {
		t.Fatalf("appended last, the hooks kept:\n%s", got)
	}
	for _, value := range []string{"true", `"no"`, "null", "{}"} {
		mine := "{\n  \"permissions\": {\n    \"allow\": []\n  },\n  \"autoMemoryEnabled\": " + value + ",\n  \"model\": \"opus\"\n}\n"
		got, action, err := SetKeys([]byte(mine), true, off)
		if err != nil || action != "updated" ||
			string(got) != strings.Replace(mine, `"autoMemoryEnabled": `+value, `"autoMemoryEnabled": false`, 1) {
			t.Errorf("%s is replaced in its place:\n%s", value, got)
		}
	}
	same := "{\n  \"autoMemoryEnabled\": false,\n  \"model\": \"opus\"\n}\n"
	if got, action, err := SetKeys([]byte(same), true, off); err != nil || action != "" || got != nil {
		t.Fatalf("an equal key is no change: %q %q %v", got, action, err)
	}
	if got, action, err := SetKeys([]byte(same), true, nil); err != nil || action != "" || got != nil {
		t.Fatalf("no keys is no change: %q %q %v", got, action, err)
	}
	if got, action, err := SetKeys(nil, false, nil); err != nil || action != "" || got != nil {
		t.Fatalf("no keys creates no file: %q %q %v", got, action, err)
	}
	for _, bad := range []string{"[1, 2]", "{", `{} x`} {
		if _, _, err := SetKeys([]byte(bad), true, off); err == nil {
			t.Errorf("%s is not a settings file", bad)
		}
	}
}

func TestMergeHooksAndKeys(t *testing.T) {
	off := []Setting{{Key: "autoMemoryEnabled", Value: false}}
	created, action, err := MergeHooksAndKeys(nil, false, want, events, ours, off)
	if err != nil || action != "created" || !Registers(created, ours) ||
		!strings.HasSuffix(string(created), "    ]\n  },\n  \"autoMemoryEnabled\": false\n}\n") {
		t.Fatalf("a new file gets the hooks and the key last: %q %v\n%s", action, err, created)
	}
	if got, action, err := MergeHooksAndKeys(created, true, want, events, ours, off); err != nil || action != "" ||
		got != nil {
		t.Fatalf("a file with both is no change: %q %q %v", got, action, err)
	}
	hooks, _, _ := MergeHooks(nil, false, want, events, ours)
	if got, action, err := MergeHooksAndKeys(hooks, true, want, events, ours, off); err != nil ||
		action != "updated" || string(got) != string(created) {
		t.Fatalf("only the key changes: %q %v\n%s", action, err, got)
	}
	first := "{\n  \"autoMemoryEnabled\": false,\n  \"hooks\": {\n    \"Stop\": [\n      {\"hooks\": [{\"type\": \"command\", \"command\": \"polix old\"}]}\n    ]\n  }\n}\n"
	got, action, err := MergeHooksAndKeys([]byte(first), true, want, events, ours, off)
	if err != nil || action != "updated" ||
		!strings.HasPrefix(string(got), "{\n  \"autoMemoryEnabled\": false,\n  \"hooks\": {") ||
		strings.Count(string(got), "autoMemoryEnabled") != 1 || strings.Contains(string(got), "polix old") {
		t.Fatalf("only the hooks change, the key in its place: %q %v\n%s", action, err, got)
	}
	only, _, _ := MergeHooks(nil, false, want, events, ours)
	if data, action, err := MergeHooksAndKeys(only, true, nil, events, ours, nil); err != nil ||
		action != "removed" || data != nil {
		t.Fatalf("without hooks nor keys the file goes: %q %q %v", data, action, err)
	}
	if data, action, err := MergeHooksAndKeys(created, true, nil, events, ours, nil); err != nil ||
		action != "updated" || string(data) != "{\n  \"autoMemoryEnabled\": false\n}\n" {
		t.Fatalf("removing the hooks keeps the key: %q %v\n%s", action, err, data)
	}
	if data, action, err := MergeHooksAndKeys(nil, false, nil, events, ours, nil); err != nil || action != "" ||
		data != nil {
		t.Fatalf("nothing to write creates no file: %q %q %v", data, action, err)
	}
	for _, bad := range []string{"[1, 2]", "{", `{"hooks": 1}`} {
		if _, _, err := MergeHooksAndKeys([]byte(bad), true, want, events, ours, off); err == nil {
			t.Errorf("%s is not a settings file", bad)
		}
	}
}
