package graph

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
)

// parseWorkflow loads src as the polix.yaml of a temporary root through the loader of polix.yaml, as cmd/polix
// builds it: the workflow and the error of loading it.
func parseWorkflow(t *testing.T, src string) (workflow.Workflow, error) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, workflow.File), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := workflowload.Command()(context.Background(), filepath.Join(root, workflow.File), "dev")
	return loaded.Workflow, loaded.Err
}

func TestDecideWithoutFile(t *testing.T) {
	if out := Decide(workflow.Workflow{Version: 1}, false); !out.Missing || out.Mermaid != "" {
		t.Fatalf("got %+v", out)
	}
}

func TestDecideDrawsEventsInOrder(t *testing.T) {
	wf := workflow.Workflow{Version: 1, On: map[string][]workflow.Entry{
		workflow.Stop: {
			{When: workflow.Cond{"changed": "src/**", "branch": "^f"}, Unless: workflow.Cond{"finished": "polix:reviewer"},
				Action: workflow.Action{Block: &workflow.Block{Message: "review"}}},
			{Action: workflow.Action{Run: &workflow.Run{Command: []string{"go", "test"}, Always: true}}},
		},
		workflow.PreEdit: {{Action: workflow.Action{Block: &workflow.Block{Message: `no "x"`, Once: true}}}},
		workflow.PostEdit: {
			{Action: workflow.Action{Run: &workflow.Run{Rules: "{path}"}}},
			{When: workflow.Cond{"not": workflow.Cond{"branch": "main"}}, Action: workflow.Action{Inject: &workflow.Inject{Rules: "{path}", Always: true}}},
		},
		workflow.PostCommit: {{Action: workflow.Action{Notify: &workflow.Notify{Message: "done"}}}},
		workflow.PreCommit:  {{Action: workflow.Action{Block: &workflow.Block{Message: "no"}}}},
		workflow.Start: {
			{Action: workflow.Action{Inject: &workflow.Inject{Text: "hola"}}},
			{Action: workflow.Action{Inject: &workflow.Inject{File: "AGENTS.md"}}},
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
		"  subgraph pre_commit\n" +
		"    n5[\"block: no\"]\n" +
		"  end\n" +
		"  subgraph post_commit\n" +
		"    n6[\"notify: done\"]\n" +
		"  end\n" +
		"  subgraph stop\n" +
		"    n7[\"when branch=^f, changed=src/**<br/>unless finished=polix:reviewer<br/>block: review\"]\n" +
		"    n8[\"run go test (always)\"]\n" +
		"    n7 --> n8\n" +
		"  end\n"
	if out := Decide(wf, true); out.Missing || out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// AC16: the protected files and the forbidden commands are drawn in a subgraph guardrails before the events, an
// expanded entry names its shortcut, and an entry written in on names none.
func TestDecideGuardrailsAndOrigin(t *testing.T) {
	wf, err := parseWorkflow(t, `version: 1
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
  run_on_stop:
    - run: "go test ./..."
      changed: "*.go"
on:
  pre_edit:
    - when: { path: "secrets/**" }
      action: { block: "Secrets." }
    - action: { block: "Written." }
`)
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
		"    n7[\"when agents_are_running=false, changed=*.go<br/>run go test ./...<br/>from guardrails.run_on_stop\"]\n" +
		"    n8[\"when agents_are_running=true, changed=[*.go]<br/>notify: run_on_stop waits for the subagents still " +
		"running: its checks run on the next stop after they end. (always)<br/>from guardrails.run_on_stop\"]\n" +
		"    n7 --> n8\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// AC22: the entry a polix.yaml with githooks expands into is drawn first in start, from githooks; the entries of
// githooks themselves are not drawn.
func TestDecideGithooksInstall(t *testing.T) {
	wf, err := parseWorkflow(t, `version: 1
on:
  start:
    - action: { inject: hola }
githooks:
  pre_commit:
    - when: { branch: "^main$" }
      action: { block: "No se commitea en main" }
`)
	if err != nil {
		t.Fatal(err)
	}
	want := "flowchart TD\n" +
		"  subgraph start\n" +
		"    n0[\"run polix install --githooks (always)<br/>from githooks\"]\n" +
		"    n1[\"inject: hola\"]\n" +
		"    n0 --> n1\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// AC9: a notify is drawn with its message, and with always when it shows every time.
func TestDecideNotify(t *testing.T) {
	wf := workflow.Workflow{Version: 1, On: map[string][]workflow.Entry{
		workflow.Prompt: {
			{When: workflow.Cond{"branch": "^main$"}, Action: workflow.Action{Notify: &workflow.Notify{
				Message: `Estás en "main"`}}},
			{Action: workflow.Action{Notify: &workflow.Notify{Message: "Rama {branch}", Always: true}}},
		},
	}}
	want := "flowchart TD\n" +
		"  subgraph prompt\n" +
		"    n0[\"when branch=^main$<br/>notify: Estás en 'main'\"]\n" +
		"    n1[\"notify: Rama {branch} (always)\"]\n" +
		"    n0 --> n1\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

func TestDecideEmpty(t *testing.T) {
	if out := Decide(workflow.Workflow{Version: 1}, true); out.Mermaid != "flowchart TD\n" {
		t.Fatalf("got %q", out.Mermaid)
	}
}
