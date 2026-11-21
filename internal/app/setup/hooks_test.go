package setup

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/hook"
)

func TestHookHandlerPolix(t *testing.T) {
	for _, c := range []string{"polix hook claude", "polix update --hook", "polix update --hook --flag-of-a-later-release",
		"polix update --hook; polix doctor --hook"} {
		if !(HookHandler{Command: c}).Polix() {
			t.Errorf("%q is a command of polix", c)
		}
	}
	if (HookHandler{Command: "polix update --check"}).Polix() || (HookHandler{Command: "polix update --hookx"}).Polix() {
		t.Error("only the session start command is the update of polix")
	}
	if !(HookHandler{Command: " polix update --hook; polix doctor --hook "}).Legacy() ||
		(HookHandler{Command: UpdateCommand}).Legacy() {
		t.Error("the legacy command")
	}
}

// The hooks give polix more time than the run budget of a call, so the agent never kills a gate in the middle.
func TestHookTimeouts(t *testing.T) {
	for _, agent := range []string{Claude, Codex} {
		for ev, groups := range (Setup{}).Hooks(agent) {
			for _, g := range groups {
				for _, h := range g.Handlers {
					if strings.HasPrefix(h.Command, "polix hook") && h.Timeout <= hook.RunBudget.Seconds() {
						t.Errorf("%s %s: timeout %v", agent, ev, h.Timeout)
					}
				}
			}
		}
	}
}

// Every handler polix registers, the update one included, shows polix while it runs, in both agents.
func TestHookStatus(t *testing.T) {
	for _, agent := range []string{Claude, Codex} {
		hooks := (Setup{}).Hooks(agent)
		if len(hooks) == 0 {
			t.Fatalf("%s: no hooks", agent)
		}
		for ev, groups := range hooks {
			for _, g := range groups {
				for _, h := range g.Handlers {
					if h.Status != StatusMessage || StatusMessage != "polix" {
						t.Errorf("%s %s %q: status %q", agent, ev, h.Command, h.Status)
					}
				}
			}
		}
	}
}

func TestHookEvents(t *testing.T) {
	if _, ok := (Setup{}).Hooks(Claude)["PostToolUseFailure"]; !ok {
		t.Fatal("Claude Code gets PostToolUseFailure")
	}
	if _, ok := (Setup{}).Hooks(Codex)["PostToolUseFailure"]; ok {
		t.Fatal("only Claude Code gets PostToolUseFailure")
	}
	for _, agent := range []string{Claude, Codex} {
		got := (Setup{}).Hooks(agent)["SubagentStart"]
		if len(got) != 1 || got[0].Matcher != "" || len(got[0].Handlers) != 1 ||
			got[0].Handlers[0].Command != "polix hook "+agent || got[0].Handlers[0].Status != StatusMessage {
			t.Fatalf("%s gets SubagentStart: %+v", agent, got)
		}
	}
	claude := make([]string, 0, len(ClaudeHookEvents))
	for _, ev := range ClaudeHookEvents {
		claude = append(claude, ev.Name)
		if ev.Name == "SubagentStart" && len(ev.Tools) > 0 {
			t.Fatalf("SubagentStart has no tools: %q", ev.Tools)
		}
	}
	for _, events := range [][]string{HookEvents, claude} {
		if i := slices.Index(events, "UserPromptSubmit"); i < 0 || slices.Index(events, "SubagentStart") != i+1 {
			t.Fatalf("SubagentStart goes right after UserPromptSubmit: %q", events)
		}
	}
	if got := (Setup{}).Hooks(Claude)["PreToolUse"]; len(got) != 1 || got[0].Matcher != claudeTools {
		t.Fatalf("the tools polix governs: %+v", got)
	}
	start := (Setup{}).Hooks(Codex)["SessionStart"]
	if len(start) != 2 || start[1].Matcher != "startup" || start[1].Handlers[0].Command != UpdateCommand {
		t.Fatalf("the session start command: %+v", start)
	}
}

func TestHookGroupValid(t *testing.T) {
	h := HookHandler{Command: "polix hook claude", Timeout: 900}
	if h.Valid() != nil || (HookHandler{Command: " ", Timeout: 900}).Valid() == nil {
		t.Fatal("a handler needs a command")
	}
	if (HookGroup{Matcher: "Bash", Handlers: []HookHandler{h}}).Valid() != nil ||
		(HookGroup{Matcher: "Bash"}).Valid() == nil ||
		(HookGroup{Handlers: []HookHandler{h, {}}}).Valid() == nil {
		t.Fatal("a group needs valid handlers")
	}
}

func TestHooksFileValid(t *testing.T) {
	hooks := map[string][]HookGroup{"Stop": HookHandler{Command: "polix hook claude"}.Alone("")}
	if (HooksFile{Hooks: hooks}).Valid() != nil || (HooksFile{Err: errors.New("not JSON")}).Valid() != nil {
		t.Fatal("a file with hooks, or that is not JSON")
	}
	if (HooksFile{Hooks: hooks, ReadErr: errors.New("denied")}).Valid() == nil ||
		(HooksFile{Hooks: map[string][]HookGroup{"Stop": {{}}}}).Valid() == nil {
		t.Fatal("a file that cannot be read holds no hooks, and its groups are valid")
	}
}
