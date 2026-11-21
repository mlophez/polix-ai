package answer

import (
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// apply_patch carries its files in the patch, given as a string or a list of arguments, under any of its keys.
func TestCodexPaths(t *testing.T) {
	patch := "*** Begin Patch\n*** Update File: src/a.go\n*** Add File: b.go\n*** Delete File: c.go\n" +
		"*** Update File: d.go\n*** Move to: e.go\n*** End Patch\n"
	want := []string{"src/a.go", "b.go", "c.go", "d.go", "e.go"}
	for _, input := range []map[string]any{
		{"patch": patch}, {"input": patch}, {"command": []any{"apply_patch", patch}},
	} {
		if got := (codex{}).paths(input); !slices.Equal(got, want) {
			t.Errorf("paths(%v) = %q", input, got)
		}
	}
	if got := (codex{}).paths(map[string]any{"file_path": "/r/x.go"}); !slices.Equal(got, []string{"/r/x.go"}) {
		t.Errorf("file_path: %q", got)
	}
}

// A command of Codex may come as a list of arguments.
func TestCodexCommand(t *testing.T) {
	if got := (codex{}).command([]any{"terraform", "apply", 1}); got != "terraform apply 1" {
		t.Fatalf("list: %q", got)
	}
	if (codex{}).command("ls") != "ls" || (codex{}).command(nil) != "" {
		t.Fatal("string or nothing")
	}
}

// The events of Codex turn into neutral events; unknown tools give none.
func TestParseCodex(t *testing.T) {
	native, evs, err := codex{}.parse([]byte(`{"hook_event_name":"PreToolUse","tool_name":"shell","cwd":"/r",` +
		`"tool_input":{"command":["git","commit","-m","x"]}}`))
	if err != nil || native != "PreToolUse" || len(evs) != 2 || evs[0].Command != "git commit -m x" ||
		evs[1].Name != workflow.PreCommit || evs[0].Agent != agent.Codex {
		t.Fatalf("shell: %q %+v %v", native, evs, err)
	}
	_, evs, _ = codex{}.parse([]byte(`{"hook_event_name":"PostToolUse","tool_name":"exec_command",` +
		`"tool_input":{"command":"x"},"tool_response":{"exitCode":1}}`))
	if len(evs) != 1 || evs[0].Name != workflow.PostCommand || evs[0].ExitCode != 1 {
		t.Fatalf("post command: %+v", evs)
	}
	_, evs, _ = codex{}.parse([]byte(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/r",` +
		`"tool_input":{"input":"*** Update File: a.go\n*** Update File: b.go\n"}}`))
	if len(evs) != 2 || evs[0].Name != workflow.PreEdit {
		t.Fatalf("apply_patch: %+v", evs)
	}
	for _, in := range []string{`{"hook_event_name":"SessionStart"}`, `{"hook_event_name":"UserPromptSubmit"}`,
		`{"hook_event_name":"SubagentStart"}`, `{"hook_event_name":"Stop"}`, `{"hook_event_name":"SubagentStop"}`} {
		if _, evs, err := (codex{}).parse([]byte(in)); err != nil || len(evs) != 1 {
			t.Errorf("%s: %+v %v", in, evs, err)
		}
	}
	if _, evs, _ := (codex{}).parse([]byte(`{"hook_event_name":"Stop"}`)); evs[0].Name != workflow.Stop ||
		evs[0].Actor != agent.ActorSession || evs[0].InFlightKnown || len(evs[0].InFlight) != 0 {
		t.Fatalf("the stop of a turn, without the subagents in flight: %+v", evs)
	}
	if _, evs, _ := (codex{}).parse([]byte(`{"hook_event_name":"PreToolUse","tool_name":"web_search"}`)); len(evs) != 0 {
		t.Fatalf("unknown tool: %+v", evs)
	}
	if _, _, err := (codex{}).parse([]byte(`[`)); err == nil {
		t.Fatal("broken JSON is an error")
	}
}

// AC4: the start of a subagent of Codex is one subagent_start of its type, and its contexts reach the subagent as
// the additionalContext of SubagentStart.
func TestCodexSubagentStart(t *testing.T) {
	native, evs, err := codex{}.parse([]byte(`{"hook_event_name":"SubagentStart","session_id":"s1",` +
		`"agent_type":"polix:reviewer"}`))
	if err != nil || native != "SubagentStart" || len(evs) != 1 || evs[0].Name != workflow.SubagentStart ||
		evs[0].Actor != "polix:reviewer" || evs[0].Agent != agent.Codex {
		t.Fatalf("parse: %q %+v %v", native, evs, err)
	}
	stdout, _, code := codex{}.render(native, agent.Answer{}.Inform("Lee las reglas"))
	want := `{"hookSpecificOutput":{"additionalContext":"Lee las reglas","hookEventName":"SubagentStart"}}` + "\n"
	if code != agent.ExitOK || stdout != want {
		t.Fatalf("render: %d %q", code, stdout)
	}
}

// A commit in the PostToolUse of a shell tool of Codex gives post_command and post_commit when its exit code is 0,
// and only post_command when it is not.
func TestCodexPostCommit(t *testing.T) {
	for response, want := range map[string]int{`{"exitCode":0}`: 2, `{"stdout":"ok"}`: 2, `{"exitCode":1}`: 1} {
		_, evs, err := codex{}.parse([]byte(`{"hook_event_name":"PostToolUse","tool_name":"shell","cwd":"/r",` +
			`"tool_input":{"command":["git","commit","-m","x"]},"tool_response":` + response + `}`))
		if err != nil || len(evs) != want || evs[0].Name != workflow.PostCommand ||
			want == 2 && (evs[1].Name != workflow.PostCommit || evs[1].Agent != agent.Codex) {
			t.Errorf("%s: %+v %v", response, evs, err)
		}
	}
}
