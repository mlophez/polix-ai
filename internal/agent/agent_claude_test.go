package agent

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/workflow"
)

// A failed edit changed nothing, so it yields no event; a failed command reaches post_command with a non-zero exit.
func TestClaudePostToolUseFailure(t *testing.T) {
	_, evs, err := parseClaude([]byte(`{"hook_event_name":"PostToolUseFailure","tool_name":"Edit","tool_input":{"file_path":"/x/a.go"}}`))
	if err != nil || len(evs) != 0 {
		t.Fatalf("failed edit: %v %+v", err, evs)
	}
	_, evs, err = parseClaude([]byte(`{"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`))
	if err != nil || len(evs) == 0 || evs[0].Name != "post_command" || evs[0].ExitCode != 1 {
		t.Fatalf("failed command: %v %+v", err, evs)
	}
}

// Each event of Claude Code turns into its neutral event, with the actor of the subagent; a stop is always of the
// main session.
func TestParseClaude(t *testing.T) {
	cases := []struct {
		input  string
		names  []string
		actor  string
		native string
	}{
		{`{"hook_event_name":"SessionStart"}`, []string{workflow.SessionStart}, ActorSession, "SessionStart"},
		{`{"hook_event_name":"UserPromptSubmit","prompt":"hola"}`, []string{workflow.Prompt}, ActorSession, "UserPromptSubmit"},
		{`{"hook_event_name":"Stop","agent_type":"builder"}`, []string{workflow.Stop}, ActorSession, "Stop"},
		{`{"hook_event_name":"SubagentStop","agent_type":"reviewer"}`, []string{workflow.SubagentStop}, "reviewer", "SubagentStop"},
		{`{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"/r/a.go"}}`, []string{workflow.PreEdit}, ActorSession, "PreToolUse"},
		{`{"hook_event_name":"PostToolUse","tool_name":"NotebookEdit","tool_input":{"notebook_path":"/r/n.ipynb"}}`, []string{workflow.PostEdit}, ActorSession, "PostToolUse"},
		{`{"hook_event_name":"PreToolUse","tool_name":"Bash","agent_type":"builder","tool_input":{"command":"git commit -m x"}}`, []string{workflow.PreCommand, workflow.PreCommit}, "builder", "PreToolUse"},
		{`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"just test"},"tool_response":{"exit_code":2}}`, []string{workflow.PostCommand}, ActorSession, "PostToolUse"},
		{`{"hook_event_name":"PreToolUse","tool_name":"Read"}`, nil, "", "PreToolUse"},
		{`{"hook_event_name":"Notification"}`, nil, "", "Notification"},
	}
	for _, c := range cases {
		native, evs, err := parseClaude([]byte(c.input))
		if err != nil || native != c.native || len(evs) != len(c.names) {
			t.Errorf("%s: %q %+v %v", c.input, native, evs, err)
			continue
		}
		for i, ev := range evs {
			if ev.Name != c.names[i] || ev.Actor != c.actor || ev.Agent != Claude || ev.Valid() != nil {
				t.Errorf("%s: event %+v", c.input, ev)
			}
		}
	}
	_, evs, _ := parseClaude([]byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"x"},"tool_response":{"exit_code":2}}`))
	if evs[0].ExitCode != 2 || evs[0].Command != "x" {
		t.Errorf("post_command: %+v", evs[0])
	}
	_, evs, _ = parseClaude([]byte(`{"hook_event_name":"PostToolUse","tool_name":"NotebookEdit","tool_input":{"notebook_path":"/r/n.ipynb"}}`))
	if evs[0].AbsPath != "/r/n.ipynb" {
		t.Errorf("notebook path: %+v", evs[0])
	}
	if _, _, err := parseClaude([]byte(`{`)); err == nil {
		t.Error("broken JSON is an error")
	}
}
