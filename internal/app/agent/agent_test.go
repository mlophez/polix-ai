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
