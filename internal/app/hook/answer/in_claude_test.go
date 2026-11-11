package answer

import (
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// A failed edit changed nothing, so it yields no event; a failed command reaches post_command with a non-zero exit.
func TestClaudePostToolUseFailure(t *testing.T) {
	_, evs, err := claude{}.parse([]byte(`{"hook_event_name":"PostToolUseFailure","tool_name":"Edit","tool_input":{"file_path":"/x/a.go"}}`))
	if err != nil || len(evs) != 0 {
		t.Fatalf("failed edit: %v %+v", err, evs)
	}
	_, evs, err = claude{}.parse([]byte(`{"hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`))
	if err != nil || len(evs) == 0 || evs[0].Name != "post_command" || evs[0].ExitCode != 1 {
		t.Fatalf("failed command: %v %+v", err, evs)
	}
}

// The id Claude Code gives the call of a tool lands in ToolUse before the command, after it and after it failed, so
// the snapshot of a command pairs with its end; an event without it has none.
func TestClaudeToolUse(t *testing.T) {
	for _, native := range []string{"PreToolUse", "PostToolUse", "PostToolUseFailure"} {
		_, evs, err := claude{}.parse([]byte(`{"hook_event_name":"` + native +
			`","tool_name":"Bash","tool_use_id":"toolu_1","tool_input":{"command":"git commit -m x"}}`))
		if err != nil || len(evs) == 0 {
			t.Fatalf("%s: %+v %v", native, evs, err)
		}
		for _, ev := range evs {
			if ev.ToolUse != "toolu_1" {
				t.Errorf("%s: event %+v", native, ev)
			}
		}
	}
	_, evs, err := claude{}.parse([]byte(`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"}}`))
	if err != nil || len(evs) != 1 || evs[0].ToolUse != "" {
		t.Fatalf("without an id: %+v %v", evs, err)
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
		{`{"hook_event_name":"SessionStart"}`, []string{workflow.Start}, agent.ActorSession, "SessionStart"},
		{`{"hook_event_name":"UserPromptSubmit","prompt":"hola"}`, []string{workflow.Prompt}, agent.ActorSession, "UserPromptSubmit"},
		{`{"hook_event_name":"SubagentStart","agent_type":"polix:reviewer"}`, []string{workflow.SubagentStart}, "polix:reviewer", "SubagentStart"},
		{`{"hook_event_name":"Stop","agent_type":"builder"}`, []string{workflow.Stop}, agent.ActorSession, "Stop"},
		{`{"hook_event_name":"SubagentStop","agent_type":"reviewer"}`, []string{workflow.SubagentStop}, "reviewer", "SubagentStop"},
		{`{"hook_event_name":"PreToolUse","tool_name":"Write","tool_input":{"file_path":"/r/a.go"}}`, []string{workflow.PreEdit}, agent.ActorSession, "PreToolUse"},
		{`{"hook_event_name":"PostToolUse","tool_name":"NotebookEdit","tool_input":{"notebook_path":"/r/n.ipynb"}}`, []string{workflow.PostEdit}, agent.ActorSession, "PostToolUse"},
		{`{"hook_event_name":"PreToolUse","tool_name":"Bash","agent_type":"builder","tool_input":{"command":"git commit -m x"}}`, []string{workflow.PreCommand, workflow.PreCommit}, "builder", "PreToolUse"},
		{`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"just test"},"tool_response":{"exit_code":2}}`, []string{workflow.PostCommand}, agent.ActorSession, "PostToolUse"},
		{`{"hook_event_name":"PreToolUse","tool_name":"Read"}`, nil, "", "PreToolUse"},
		{`{"hook_event_name":"Notification"}`, nil, "", "Notification"},
	}
	for _, c := range cases {
		native, evs, err := claude{}.parse([]byte(c.input))
		if err != nil || native != c.native || len(evs) != len(c.names) {
			t.Errorf("%s: %q %+v %v", c.input, native, evs, err)
			continue
		}
		for i, ev := range evs {
			if ev.Name != c.names[i] || ev.Actor != c.actor || ev.Agent != agent.Claude || ev.Valid() != nil {
				t.Errorf("%s: event %+v", c.input, ev)
			}
		}
	}
	_, evs, _ := claude{}.parse([]byte(`{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"x"},"tool_response":{"exit_code":2}}`))
	if evs[0].ExitCode != 2 || evs[0].Command != "x" {
		t.Errorf("post_command: %+v", evs[0])
	}
	_, evs, _ = claude{}.parse([]byte(`{"hook_event_name":"PostToolUse","tool_name":"NotebookEdit","tool_input":{"notebook_path":"/r/n.ipynb"}}`))
	if evs[0].AbsPath != "/r/n.ipynb" {
		t.Errorf("notebook path: %+v", evs[0])
	}
	if _, _, err := (claude{}).parse([]byte(`{`)); err == nil {
		t.Error("broken JSON is an error")
	}
}

// The agent_id of SubagentStart, SubagentStop and a tool call inside a subagent, and the source of SessionStart,
// reach the event; without them, or with a value that is not a text, they are empty and the input still parses.
// The main session, a Stop included, has no agent_id, and only a start has a source.
func TestClaudeAgentIDAndSource(t *testing.T) {
	cases := []struct {
		input   string
		agentID string
		source  string
	}{
		{`{"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"builder"}`, "a1", ""},
		{`{"hook_event_name":"SubagentStop","agent_id":"a1","agent_type":"builder"}`, "a1", ""},
		{`{"hook_event_name":"PreToolUse","agent_id":"a1","agent_type":"builder","tool_name":"Bash",` +
			`"tool_input":{"command":"git commit -m x"}}`, "a1", ""},
		{`{"hook_event_name":"SessionStart","source":"compact"}`, "", "compact"},
		{`{"hook_event_name":"SessionStart"}`, "", ""},
		{`{"hook_event_name":"SessionStart","source":7}`, "", ""},
		{`{"hook_event_name":"SubagentStop","agent_type":"builder"}`, "", ""},
		{`{"hook_event_name":"SubagentStop","agent_id":["a1"],"agent_type":"builder"}`, "", ""},
		{`{"hook_event_name":"Stop","agent_id":"a1","source":"startup"}`, "", ""},
		{`{"hook_event_name":"UserPromptSubmit","agent_id":"a1","prompt":"x"}`, "", ""},
	}
	for _, c := range cases {
		_, evs, err := claude{}.parse([]byte(c.input))
		if err != nil || len(evs) == 0 {
			t.Errorf("%s: %+v %v", c.input, evs, err)
			continue
		}
		for _, ev := range evs {
			if ev.AgentID != c.agentID || ev.Source != c.source || ev.Valid() != nil {
				t.Errorf("%s: event %+v", c.input, ev)
			}
		}
	}
}

// The background_tasks of Stop and SubagentStop give the subagents in flight, the other tasks left out, whatever their
// status; without the array, or with a value that is not an array of tasks, they are unknown, and an empty array
// is known and empty. The input parses in every case, and the other events never carry them.
func TestClaudeBackgroundTasks(t *testing.T) {
	tasks := `[{"id":"a1","type":"subagent","agent_type":"builder","status":"running"},` +
		`{"id":"b1","type":"shell","status":"running"},{"id":"a2","type":"subagent","agent_type":"reviewer",` +
		`"status":"completed"}]`
	cases := []struct {
		input string
		known bool
		ids   string
	}{
		{`{"hook_event_name":"Stop","background_tasks":` + tasks + `}`, true, "a1:builder,a2:reviewer"},
		{`{"hook_event_name":"SubagentStop","agent_id":"a1","agent_type":"builder","background_tasks":` + tasks + `}`,
			true, "a1:builder,a2:reviewer"},
		{`{"hook_event_name":"Stop","background_tasks":[]}`, true, ""},
		{`{"hook_event_name":"Stop"}`, false, ""},
		{`{"hook_event_name":"Stop","background_tasks":null}`, false, ""},
		{`{"hook_event_name":"Stop","background_tasks":"a1"}`, false, ""},
		{`{"hook_event_name":"Stop","background_tasks":["a1"]}`, false, ""},
		{`{"hook_event_name":"Stop","background_tasks":[{"type":"subagent"}]}`, false, ""},
		{`{"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"builder","background_tasks":` + tasks + `}`,
			false, ""},
	}
	for _, c := range cases {
		_, evs, err := claude{}.parse([]byte(c.input))
		if err != nil || len(evs) != 1 {
			t.Errorf("%s: %+v %v", c.input, evs, err)
			continue
		}
		var ids []string
		for _, s := range evs[0].InFlight {
			ids = append(ids, s.ID+":"+s.Type)
		}
		if evs[0].InFlightKnown != c.known || strings.Join(ids, ",") != c.ids || evs[0].Valid() != nil {
			t.Errorf("%s: event %+v", c.input, evs[0])
		}
	}
}

// AC4: the start of a subagent of Claude Code is one subagent_start of its type, and its contexts reach the subagent
// as the additionalContext of SubagentStart.
func TestClaudeSubagentStart(t *testing.T) {
	native, evs, err := claude{}.parse([]byte(`{"hook_event_name":"SubagentStart","session_id":"s1",` +
		`"agent_id":"a1","agent_type":"polix:reviewer"}`))
	if err != nil || native != "SubagentStart" || len(evs) != 1 || evs[0].Name != workflow.SubagentStart ||
		evs[0].Actor != "polix:reviewer" || evs[0].Session != "s1" {
		t.Fatalf("parse: %q %+v %v", native, evs, err)
	}
	stdout, _, code := claude{}.render(native, agent.Answer{}.Inform("Lee las reglas"))
	want := `{"hookSpecificOutput":{"additionalContext":"Lee las reglas","hookEventName":"SubagentStart"}}` + "\n"
	if code != agent.ExitOK || stdout != want {
		t.Fatalf("render: %d %q", code, stdout)
	}
}
