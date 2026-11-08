package workflow

import "testing"

func TestGraphWorkflowWithoutFile(t *testing.T) {
	if out := GraphWorkflow(Workflow{Version: 1}, false); !out.Missing || out.Mermaid != "" {
		t.Fatalf("got %+v", out)
	}
}

func TestGraphWorkflowDrawsEventsInOrder(t *testing.T) {
	wf := Workflow{Version: 1, On: map[string][]Entry{
		Stop: {
			{When: Cond{"changed": "src/**", "branch": "^f"}, Unless: Cond{"finished": "polix:reviewer"},
				Action: Action{Block: &Block{Message: "review"}}},
			{Action: Action{Run: &Run{Command: []string{"go", "test"}, Always: true}}},
		},
		PreEdit: {{Action: Action{Block: &Block{Message: `no "x"`, Once: true}}}},
		PostEdit: {
			{Action: Action{Run: &Run{Rules: "{path}"}}},
			{When: Cond{"not": Cond{"branch": "main"}}, Action: Action{Inject: &Inject{Rules: "{path}", Always: true}}},
		},
		Start: {
			{Action: Action{Inject: &Inject{Text: "hola"}}},
			{Action: Action{Inject: &Inject{File: "AGENTS.md"}}},
		},
	}}
	want := "flowchart TD\n" +
		"  subgraph start\n" +
		"    n0[\"inject: hola\"]\n" +
		"    n1[\"inject file AGENTS.md\"]\n" +
		"    n0 --> n1\n" +
		"  end\n" +
		"  subgraph pre_edit\n" +
		"    n2[\"block: no 'x' (once)\"]\n" +
		"  end\n" +
		"  subgraph post_edit\n" +
		"    n3[\"run rules {path}\"]\n" +
		"    n4[\"when not=map[branch:main]<br/>inject rules {path} (always)\"]\n" +
		"    n3 --> n4\n" +
		"  end\n" +
		"  subgraph stop\n" +
		"    n5[\"when branch=^f, changed=src/**<br/>unless finished=polix:reviewer<br/>block: review\"]\n" +
		"    n6[\"run go test (always)\"]\n" +
		"    n5 --> n6\n" +
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
