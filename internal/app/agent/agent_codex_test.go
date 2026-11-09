package agent

import (
	"slices"
	"testing"

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
		if got := codexPaths(input); !slices.Equal(got, want) {
			t.Errorf("codexPaths(%v) = %q", input, got)
		}
	}
	if got := codexPaths(map[string]any{"file_path": "/r/x.go"}); !slices.Equal(got, []string{"/r/x.go"}) {
		t.Errorf("file_path: %q", got)
	}
}

// A command of Codex may come as a list of arguments.
func TestCodexCommand(t *testing.T) {
	if got := codexCommand([]any{"terraform", "apply", 1}); got != "terraform apply 1" {
		t.Fatalf("list: %q", got)
	}
	if codexCommand("ls") != "ls" || codexCommand(nil) != "" {
		t.Fatal("string or nothing")
	}
}

// The events of Codex turn into neutral events; unknown tools give none.
func TestParseCodex(t *testing.T) {
	native, evs, err := parseCodex([]byte(`{"hook_event_name":"PreToolUse","tool_name":"shell","cwd":"/r",` +
		`"tool_input":{"command":["git","commit","-m","x"]}}`))
	if err != nil || native != "PreToolUse" || len(evs) != 2 || evs[0].Command != "git commit -m x" ||
		evs[1].Name != workflow.PreCommit || evs[0].Agent != Codex {
		t.Fatalf("shell: %q %+v %v", native, evs, err)
	}
	_, evs, _ = parseCodex([]byte(`{"hook_event_name":"PostToolUse","tool_name":"exec_command",` +
		`"tool_input":{"command":"x"},"tool_response":{"exitCode":1}}`))
	if len(evs) != 1 || evs[0].Name != workflow.PostCommand || evs[0].ExitCode != 1 {
		t.Fatalf("post command: %+v", evs)
	}
	_, evs, _ = parseCodex([]byte(`{"hook_event_name":"PreToolUse","tool_name":"apply_patch","cwd":"/r",` +
		`"tool_input":{"input":"*** Update File: a.go\n*** Update File: b.go\n"}}`))
	if len(evs) != 2 || evs[0].Name != workflow.PreEdit {
		t.Fatalf("apply_patch: %+v", evs)
	}
	for _, in := range []string{`{"hook_event_name":"SessionStart"}`, `{"hook_event_name":"UserPromptSubmit"}`,
		`{"hook_event_name":"Stop"}`, `{"hook_event_name":"SubagentStop"}`} {
		if _, evs, err := parseCodex([]byte(in)); err != nil || len(evs) != 1 {
			t.Errorf("%s: %+v %v", in, evs, err)
		}
	}
	if _, evs, _ := parseCodex([]byte(`{"hook_event_name":"Stop"}`)); evs[0].Name != workflow.Stop ||
		evs[0].Actor != ActorSession {
		t.Fatalf("the stop of a turn: %+v", evs)
	}
	if _, evs, _ := parseCodex([]byte(`{"hook_event_name":"PreToolUse","tool_name":"web_search"}`)); len(evs) != 0 {
		t.Fatalf("unknown tool: %+v", evs)
	}
	if _, _, err := parseCodex([]byte(`[`)); err == nil {
		t.Fatal("broken JSON is an error")
	}
}
