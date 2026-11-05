package workflow

import "testing"

func TestGraphWorkflowWithoutFile(t *testing.T) {
	if out := GraphWorkflow(Workflow{Version: 1}, false); !out.Missing || out.Mermaid != "" {
		t.Fatalf("got %+v", out)
	}
}

func TestGraphWorkflowDrawsEventsInOrder(t *testing.T) {
	deny, require, record := `no "x"`, "review", "reviewed"
	wf := Workflow{Version: 1, On: map[string][]Entry{
		Stop: {
			{If: Cond{"changed": "src/**", "branch": "^f"}, Unless: Cond{"done": "reviewed"}, Require: &require},
			{Run: &Run{Command: []string{"go", "test"}, Record: "tested"}, Mode: ModeWarn},
		},
		PreEdit:  {{Deny: &deny, Mode: ModeEnforce}},
		PostEdit: {{Run: &Run{Rules: "{path}"}}, {Record: &record, If: Cond{"not": Cond{"branch": "main"}}}},
	}}
	want := "flowchart TD\n" +
		"  subgraph pre_edit\n" +
		"    n0[\"deny: no 'x'\"]\n" +
		"  end\n" +
		"  subgraph post_edit\n" +
		"    n1[\"run rules {path}\"]\n" +
		"    n2[\"if not=map[branch:main]<br/>record: reviewed\"]\n" +
		"    n1 --> n2\n" +
		"  end\n" +
		"  subgraph stop\n" +
		"    n3[\"if branch=^f, changed=src/**<br/>unless done=reviewed<br/>require: review\"]\n" +
		"    n4[\"run go test → record tested (warn)\"]\n" +
		"    n3 --> n4\n" +
		"  end\n"
	if out := GraphWorkflow(wf, true); out.Missing || out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

func TestGraphWorkflowEmpty(t *testing.T) {
	if out := GraphWorkflow(Workflow{Version: 1}, true); out.Mermaid != "flowchart TD\n" {
		t.Fatalf("got %q", out.Mermaid)
	}
}
