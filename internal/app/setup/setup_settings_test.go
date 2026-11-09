package setup

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// The example hooks files of the repository are, byte for byte, what install writes for each agent in an empty
// repository.
func TestHooksExamples(t *testing.T) {
	for agent, example := range map[string]string{Claude: "claude-settings.json", Codex: "codex-hooks.json"} {
		got, action, err := RegisterHooks(nil, false, agent)
		if err != nil || action != "created" {
			t.Fatal(action, err)
		}
		if want := testenv.Read(t, filepath.Join("..", "..", "..", "examples", example)); want != string(got) {
			t.Errorf("examples/%s differs from what install writes:\n%s", example, got)
		}
	}
}

// The rest of the file keeps its order, the hooks of the user stay and the one of polix is replaced in place;
// a second registration changes nothing; unregistering leaves the rest.
func TestRegisterHooksKeepsTheFile(t *testing.T) {
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
	got, action, err := RegisterHooks([]byte(mine), true, Claude)
	if err != nil || action != "updated" {
		t.Fatal(action, err)
	}
	s := string(got)
	if p, h, m := strings.Index(s, `"permissions"`), strings.Index(s, `"hooks"`), strings.Index(s, `"model"`); p >= h || h >= m {
		t.Fatalf("the order of the keys is kept:\n%s", s)
	}
	h, _ := readHooks(got)
	if len(h["Stop"]) != 2 || h["Stop"][0].Hooks[0].Command != "notify-send done" ||
		h["Stop"][1].Hooks[0].Command != "polix hook claude" || h["Stop"][1].Hooks[0].Timeout != HookTimeout {
		t.Fatalf("the hooks of the user stay and polix is replaced in place: %+v", h["Stop"])
	}
	if len(h) != len(hooks(Claude)) || h["PreToolUse"][0].Matcher != claudeTools {
		t.Fatalf("every event of polix: %+v", h)
	}
	if _, action, _ := RegisterHooks(got, true, Claude); action != "" {
		t.Fatalf("a second registration changes nothing: %q", action)
	}
	rest, action, err := UnregisterHooks(got, true)
	var doc map[string]any
	if err != nil || action != "updated" || json.Unmarshal(rest, &doc) != nil || doc["model"] != "opus" || Registered(rest) {
		t.Fatalf("unregister leaves the rest: %q %v\n%s", action, err, rest)
	}
	only, _, _ := RegisterHooks(nil, false, Codex)
	if data, action, _ := UnregisterHooks(only, true); action != "removed" || data != nil {
		t.Fatalf("a file of polix alone is removed: %q", action)
	}
	if _, action, _ := UnregisterHooks(nil, false); action != "" {
		t.Fatalf("no file, nothing to do: %q", action)
	}
}

// The session start command of an earlier build is replaced in place, not kept next to the new one.
func TestRegisterHooksReplacesALegacyCommand(t *testing.T) {
	got, _, err := RegisterHooks([]byte(`{"hooks": {"SessionStart": [
  {"hooks": [{"type": "command", "command": "polix hook claude", "timeout": 900}]},
  {"matcher": "startup", "hooks": [{"type": "command", "command": "polix update --hook; polix doctor --hook", "timeout": 180}]}
]}}`), true, Claude)
	if err != nil || strings.Contains(string(got), "polix doctor --hook") || strings.Count(string(got), `"`+UpdateCommand+`"`) != 1 {
		t.Fatalf("the legacy command is replaced: %v\n%s", err, got)
	}
}

// A group that mixes polix with another command keeps the other command, and polix ends once on the event.
func TestRegisterHooksMixedGroup(t *testing.T) {
	mixed := `{"hooks": {"Stop": [{"hooks": [
  {"type": "command", "command": "polix hook claude"},
  {"type": "command", "command": "notify-send done"}
]}]}}`
	got, _, err := RegisterHooks([]byte(mixed), true, Claude)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := readHooks(got)
	polix, other := 0, 0
	for _, g := range h["Stop"] {
		for _, c := range g.Hooks {
			switch c.Command {
			case "polix hook claude":
				polix++
			case "notify-send done":
				other++
			}
		}
	}
	if polix != 1 || other != 1 {
		t.Fatalf("one polix and the other command: %+v", h["Stop"])
	}
	rest, _, _ := UnregisterHooks(got, true)
	h, _ = readHooks(rest)
	if Registered(rest) || len(h["Stop"]) != 1 || h["Stop"][0].Hooks[0].Command != "notify-send done" {
		t.Fatalf("unregister leaves the other command: %+v", h)
	}
}

// Every group of hooks is written on one line.
func TestHooksGroupsInline(t *testing.T) {
	got, _, _ := RegisterHooks(nil, false, Claude)
	for l := range strings.SplitSeq(string(got), "\n") {
		inline := strings.HasPrefix(l, "      { ") && strings.Contains(l, `"hooks": [{ "type": "command"`)
		if strings.Contains(l, `"command"`) && !inline {
			t.Fatalf("a group spans lines:\n%s", got)
		}
	}
}

// A broken file is an error that names the part that is broken.
func TestRegisterHooksBrokenFile(t *testing.T) {
	for data, want := range map[string]string{"[]": "not a JSON object", `{"hooks": 1}`: "hooks: not a JSON object",
		`{"hooks": {"Stop": 1}}`: "hooks.Stop: "} {
		if _, _, err := RegisterHooks([]byte(data), true, Claude); err == nil || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%s: %v", data, err)
		}
	}
}
