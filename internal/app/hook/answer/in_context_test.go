package answer

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/agent"
	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// systemMessage returns the systemMessage of the JSON stdout of a hook, "" when stdout is empty or holds none.
func systemMessage(t *testing.T, stdout string) string {
	t.Helper()
	if stdout == "" {
		return ""
	}
	var out struct {
		SystemMessage string `json:"systemMessage"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %q: %v", stdout, err)
	}
	return out.SystemMessage
}

// nativeEvents are the native events of each agent that polix reads as each neutral event (see parse).
var nativeEvents = map[string]map[string][]string{
	agent.Claude: {
		workflow.Start:         {"SessionStart"},
		workflow.Prompt:        {"UserPromptSubmit"},
		workflow.SubagentStart: {"SubagentStart"},
		workflow.PreEdit:       {"PreToolUse"},
		workflow.PostEdit:      {"PostToolUse"},
		workflow.PreCommand:    {"PreToolUse"},
		workflow.PostCommand:   {"PostToolUse", "PostToolUseFailure"},
		workflow.PreCommit:     {"PreToolUse"},
		workflow.PostCommit:    {"PostToolUse"},
		workflow.SubagentStop:  {"SubagentStop"},
		workflow.Stop:          {"Stop"},
	},
	agent.Codex: {
		workflow.Start:         {"SessionStart"},
		workflow.Prompt:        {"UserPromptSubmit"},
		workflow.SubagentStart: {"SubagentStart"},
		workflow.PreEdit:       {"PreToolUse"},
		workflow.PostEdit:      {"PostToolUse"},
		workflow.PreCommand:    {"PreToolUse"},
		workflow.PostCommand:   {"PostToolUse"},
		workflow.PreCommit:     {"PreToolUse"},
		workflow.PostCommit:    {"PostToolUse"},
		workflow.SubagentStop:  {"SubagentStop"},
		workflow.Stop:          {"Stop"},
	},
}

// eventAnswers returns the answers the verbs of workflow.EventVerbs allow on the event name, by kind: contexts from
// an inject or a failing run on an event that already happened, a block from a block or a failing run on any other,
// and messages for the user on every event, with none of them besides.
func eventAnswers(name string) map[string]agent.Answer {
	verbs := workflow.EventVerbs[name]
	informed := slices.Contains(workflow.Informed, name)
	answers := map[string]agent.Answer{
		"none":     {},
		"messages": agent.Answer{}.Warn("a").Warn("b"),
	}
	if slices.Contains(verbs, workflow.VerbInject) || slices.Contains(verbs, workflow.VerbRun) && informed {
		answers["contexts"] = agent.Answer{}.Inform("x<y").Inform("z")
		answers["both"] = agent.Answer{}.Inform("x<y").Warn("a")
	}
	if slices.Contains(verbs, workflow.VerbBlock) || slices.Contains(verbs, workflow.VerbRun) && !informed {
		answers["block"] = agent.Answer{}.Refuse("r").Warn("a")
		if slices.Contains(verbs, workflow.VerbInject) {
			answers["block with contexts"] = agent.Answer{}.Inform("c").Refuse("r").Warn("a")
		}
	}
	return answers
}

// AC8: for every neutral event, its native event in each agent and every answer EventVerbs allows on it, the render
// of the agent writes only the fields that agent accepts there: systemMessage on every event, and hookSpecificOutput
// with additionalContext and hookEventName only on the events of its contextEvents. stdout is empty or one JSON
// object, stderr is empty on exit 0, and a block is exit 2 with stdout empty and no message for the user.
func TestRenderFieldsOfEachAgent(t *testing.T) {
	agents := map[string]contextEvents{agent.Claude: claudeContextEvents, agent.Codex: codexContextEvents}
	for _, name := range slices.Sorted(maps.Keys(agents)) {
		ce := agents[name]
		for _, event := range workflow.Events {
			natives := nativeEvents[name][event]
			if len(natives) == 0 {
				t.Fatalf("%s: no native event for %s", name, event)
			}
			for _, native := range natives {
				for kind, ans := range eventAnswers(event) {
					t.Run(name+"/"+event+"/"+native+"/"+kind, func(t *testing.T) {
						checkRender(t, ce, native, ans)
					})
				}
			}
		}
	}
}

// checkRender checks the render of ans on the native event of an agent whose events that take context are ce.
func checkRender(t *testing.T, ce contextEvents, native string, ans agent.Answer) {
	t.Helper()
	stdout, stderr, code := ce.render(native, ans)
	if ans.Block {
		want := strings.Join(ans.Reasons, "\n\n")
		if len(ans.Contexts) > 0 {
			want += "\n\n" + strings.Join(ans.Contexts, "\n\n")
		}
		if code != agent.ExitBlock || stdout != "" || stderr != want+"\n" || strings.Contains(stderr, messagePrefix) {
			t.Fatalf("block: %q %q %d", stdout, stderr, code)
		}
		return
	}
	if code != agent.ExitOK || stderr != "" {
		t.Fatalf("exit 0 writes nothing to stderr: %q %d", stderr, code)
	}
	takes := len(ans.Contexts) > 0 && ce[native]
	if len(ans.Warnings) == 0 && !takes {
		if stdout != "" {
			t.Fatalf("nothing to write: %q", stdout)
		}
		return
	}
	if !strings.HasSuffix(stdout, "}\n") || strings.Count(stdout, "\n") != 1 {
		t.Fatalf("stdout is not one JSON line: %q", stdout)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not one JSON object: %q: %v", stdout, err)
	}
	checkMessage(t, out, ans)
	checkContext(t, out, ans, native, takes)
	for key := range out {
		if key != "systemMessage" && key != "hookSpecificOutput" {
			t.Fatalf("field %s is not accepted on %s: %s", key, native, stdout)
		}
	}
}

// checkMessage checks that the JSON object out holds the messages of ans in systemMessage, each with
// messagePrefix, and no systemMessage without them.
func checkMessage(t *testing.T, out map[string]json.RawMessage, ans agent.Answer) {
	t.Helper()
	raw, ok := out["systemMessage"]
	if len(ans.Warnings) == 0 {
		if ok {
			t.Fatalf("a systemMessage without messages: %s", raw)
		}
		return
	}
	var msg string
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	want := make([]string, 0, len(ans.Warnings))
	for _, w := range ans.Warnings {
		want = append(want, messagePrefix+w)
	}
	if msg != strings.Join(want, "\n") {
		t.Fatalf("systemMessage: %q", msg)
	}
}

// checkContext checks that the JSON object out holds hookSpecificOutput only when the native event takes the
// contexts of ans, with additionalContext and hookEventName alone.
func checkContext(t *testing.T, out map[string]json.RawMessage, ans agent.Answer, native string, takes bool) {
	t.Helper()
	raw, ok := out["hookSpecificOutput"]
	if !takes {
		if ok {
			t.Fatalf("hookSpecificOutput is not accepted here: %s", raw)
		}
		return
	}
	var body map[string]string
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"additionalContext": strings.Join(ans.Contexts, "\n\n"), "hookEventName": native}
	if !maps.Equal(body, want) {
		t.Fatalf("hookSpecificOutput: %v", body)
	}
}

// The neutral events of the table of nativeEvents are those polix reads from each native event.
func TestNativeEventsParse(t *testing.T) {
	for name, events := range nativeEvents {
		for event, natives := range events {
			for _, native := range natives {
				got := parsedEvent(t, name, native, event)
				if got != event {
					t.Errorf("%s: %s parses to %q, want %s", name, native, got, event)
				}
			}
		}
	}
}

// parsedEvent returns the neutral event the format of the agent name reads from a call of native that stands for
// event: an edit, a command or a commit for the events of tools.
func parsedEvent(t *testing.T, name, native, event string) string {
	t.Helper()
	in := map[string]any{"hook_event_name": native, "session_id": "s1", "cwd": "/repo", "agent_type": "polix:reviewer"}
	switch event {
	case workflow.PreEdit, workflow.PostEdit:
		in["tool_name"] = "Edit"
		in["tool_input"] = map[string]any{"file_path": "/repo/a.go"}
	case workflow.PreCommand, workflow.PostCommand:
		in["tool_name"] = "Bash"
		in["tool_input"] = map[string]any{"command": "ls"}
	case workflow.PreCommit, workflow.PostCommit:
		in["tool_name"] = "Bash"
		in["tool_input"] = map[string]any{"command": "git commit -m x"}
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	_, evs, err := formats[name].parse(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evs {
		if e.Name == event {
			return e.Name
		}
	}
	if len(evs) == 0 {
		return ""
	}
	return evs[0].Name
}
