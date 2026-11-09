package agent

import "testing"

// polix answers Claude Code and Codex, and no other agent.
func TestAgentValid(t *testing.T) {
	for name, valid := range map[string]bool{Claude: true, Codex: true, "cursor": false, "": false} {
		if err := (Agent{Name: name}).Valid(); (err == nil) != valid {
			t.Errorf("Agent{%q}.Valid() = %v", name, err)
		}
	}
}

// Parse and Render go through the format of the agent: PostToolUseFailure gives context to Claude Code only.
func TestAgentParseRender(t *testing.T) {
	input := []byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`)
	for _, name := range []string{Claude, Codex} {
		native, evs, err := (Agent{Name: name}).Parse(input)
		if err != nil || native != "PostToolUse" || len(evs) != 1 || evs[0].Agent != name {
			t.Errorf("%s: %q %+v %v", name, native, evs, err)
		}
	}
	ans := Answer{}.Inform("x")
	if out, _, _ := (Agent{Name: Claude}).Render("PostToolUseFailure", ans); out == "" {
		t.Error("Claude Code reads context on PostToolUseFailure")
	}
	if out, _, _ := (Agent{Name: Codex}).Render("PostToolUseFailure", ans); out != "" {
		t.Error("Codex does not read context on PostToolUseFailure")
	}
}
