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

// AC16: the protected files and the forbidden commands are drawn in a subgraph guardrails before the events, an
// expanded entry names its shortcut, and an entry written in on names none.
func TestGraphWorkflowGuardrailsAndOrigin(t *testing.T) {
	wf, err := Parse([]byte(`version: 1
guardrails:
  protected_files: [.env, AGENTS.md]
  forbidden_commands:
    - pattern: "rm -rf /*"
    - pattern: "git push --force*"
      reason: 'Force pushes are for "a person".'
  protected_branches: [main]
  format_on_edit:
    - run: "gofmt -w {path}"
      path: "*.go"
  run_before_done:
    - run: "go test ./..."
      changed: "*.go"
on:
  pre_edit:
    - when: { path: "secrets/**" }
      action: { block: "Secrets." }
    - action: { block: "Written." }
`), "dev")
	if err != nil {
		t.Fatal(err)
	}
	if p := wf.Problems(); len(p) > 0 {
		t.Fatalf("problems: %v", p)
	}
	want := "flowchart TD\n" +
		"  subgraph guardrails\n" +
		"    n0[\"protected_files: .env, AGENTS.md\"]\n" +
		"    n1[\"forbidden_commands: rm -rf /*\"]\n" +
		"    n0 --> n1\n" +
		"    n2[\"forbidden_commands: git push --force*<br/>Force pushes are for 'a person'.\"]\n" +
		"    n1 --> n2\n" +
		"  end\n" +
		"  subgraph pre_edit\n" +
		"    n3[\"when path=secrets/**<br/>block: Secrets.\"]\n" +
		"    n4[\"block: Written.\"]\n" +
		"    n3 --> n4\n" +
		"  end\n" +
		"  subgraph post_edit\n" +
		"    n5[\"when path=*.go<br/>run gofmt -w {path}<br/>from guardrails.format_on_edit\"]\n" +
		"  end\n" +
		"  subgraph pre_commit\n" +
		"    n6[\"when branch=^(?:main)$<br/>block: {branch} is protected in polix.yaml: " +
		"create a branch for this change and commit there.<br/>from guardrails.protected_branches\"]\n" +
		"  end\n" +
		"  subgraph stop\n" +
		"    n7[\"when changed=*.go<br/>run go test ./...<br/>from guardrails.run_before_done\"]\n" +
		"  end\n"
	if out := GraphWorkflow(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

func TestGraphWorkflowEmpty(t *testing.T) {
	if out := GraphWorkflow(Workflow{Version: 1}, true); out.Mermaid != "flowchart TD\n" {
		t.Fatalf("got %q", out.Mermaid)
	}
}
