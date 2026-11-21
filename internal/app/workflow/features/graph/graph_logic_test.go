package graph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowadapters "github.com/mlophez/polix-ai/internal/app/workflow/adapters"
)

// parseWorkflow loads src as the polix.yaml of a temporary root through the loader of polix.yaml, as cmd/polix
// builds it: the workflow and the error of loading it.
func parseWorkflow(t *testing.T, src string) (workflow.Workflow, error) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, workflow.File), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := (&workflowadapters.File{}).Load(context.Background(), filepath.Join(root, workflow.File), "dev")
	return loaded.Workflow, loaded.Err
}

// builtinOff is the key builtin of guardrails with every built-in good practice off, for the tests of the other
// entries and protections.
const builtinOff = "  builtin: { forbid_commit_no_verify: false, forbid_git_destructive_commands: false, " +
	"format_polix_files: false, check_polix_files: false }\n"

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
`+builtinOff+`  protected_files: [.env, AGENTS.md]
  forbidden_commands:
    - pattern: "rm -rf /*"
    - pattern: "git push --force*"
      reason: 'Force pushes are for "a person".'
  protected_branches: [main]
  format_on_edit:
    - run: "gofmt -w {path}"
      path: "*.go"
  check_on_stop:
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
		"    n7[\"when agents_are_running=false, changed=*.go<br/>run go test ./...<br/>from guardrails.check_on_stop\"]\n" +
		"    n8[\"when agents_are_running=true, changed=[*.go]<br/>notify: check_on_stop waits for the subagents still " +
		"running: its checks run on the next stop after they end. (always)<br/>from guardrails.check_on_stop\"]\n" +
		"    n7 --> n8\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// CUJ-10, FND-01: an item of check_on_stop with also_on_save is drawn on stop and on githooks.pre_commit, both from
// guardrails.check_on_stop; the entries of githooks are drawn after the events, one subgraph githooks.<event> per
// event of githooks with entries: those of check_polix_files, check_on_save and also_on_save, then the written ones.
func TestGraphGithooksAlsoOnSave(t *testing.T) {
	wf, err := parseWorkflow(t, `version: 1
guardrails:
  builtin: { forbid_commit_no_verify: false, forbid_git_destructive_commands: false, format_polix_files: false }
  check_on_stop:
    - run: go vet ./...
      changed: "*.go"
      message: Fix go vet
      also_on_save: true
  check_on_save:
    - run: go test ./...
githooks:
  commit_msg:
    - action: { block: "No message." }
  pre_commit:
    - action: { block: "Written." }
`)
	if err != nil {
		t.Fatal(err)
	}
	if p := wf.Problems(); len(p) > 0 {
		t.Fatalf("problems: %v", p)
	}
	want := "flowchart TD\n" +
		"  subgraph start\n" +
		"    n0[\"run polix install --githooks (always)<br/>from githooks\"]\n" +
		"  end\n" +
		"  subgraph stop\n" +
		"    n1[\"when agents_are_running=false, changed=*.go<br/>run go vet ./...<br/>from guardrails.check_on_stop\"]\n" +
		"    n2[\"when agents_are_running=true, changed=[*.go]<br/>notify: check_on_stop waits for the subagents still " +
		"running: its checks run on the next stop after they end. (always)<br/>from guardrails.check_on_stop\"]\n" +
		"    n1 --> n2\n" +
		"  end\n" +
		"  subgraph githooks_pre_commit [githooks.pre_commit]\n" +
		"    n3[\"when changed=[/polix.yaml ai/rules/**]<br/>run polix config check<br/>" +
		"from guardrails.builtin.check_polix_files\"]\n" +
		"    n4[\"run go test ./...<br/>from guardrails.check_on_save\"]\n" +
		"    n3 --> n4\n" +
		"    n5[\"when changed=*.go<br/>run go vet ./...<br/>from guardrails.check_on_stop\"]\n" +
		"    n4 --> n5\n" +
		"    n6[\"block: Written.\"]\n" +
		"    n5 --> n6\n" +
		"  end\n" +
		"  subgraph githooks_commit_msg [githooks.commit_msg]\n" +
		"    n7[\"block: No message.\"]\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// AC22: the entry a polix.yaml with githooks expands into is drawn first in start, from githooks, and the entries of
// githooks themselves in a subgraph githooks.<event> after the events.
func TestDecideGithooksInstall(t *testing.T) {
	wf, err := parseWorkflow(t, `version: 1
on:
  start:
    - action: { inject: hola }
githooks:
  pre_commit:
    - when: { branch: "^main$" }
      action: { block: "No se commitea en main" }
guardrails:
`+builtinOff)
	if err != nil {
		t.Fatal(err)
	}
	want := "flowchart TD\n" +
		"  subgraph start\n" +
		"    n0[\"run polix install --githooks (always)<br/>from githooks\"]\n" +
		"    n1[\"inject: hola\"]\n" +
		"    n0 --> n1\n" +
		"  end\n" +
		"  subgraph githooks_pre_commit [githooks.pre_commit]\n" +
		"    n2[\"when branch=^main$<br/>block: No se commitea en main\"]\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// CUJ-02: the two entries of guardrails.builtin.format_polix_files, which a polix.yaml gets without writing them, are
// drawn on post_edit before the entry of format_on_edit, each with its origin.
func TestGraphBuiltinFormat(t *testing.T) {
	wf, err := parseWorkflow(t, `version: 1
guardrails:
  builtin: { forbid_commit_no_verify: false, forbid_git_destructive_commands: false, check_polix_files: false }
  format_on_edit:
    - run: gofmt -w {path}
      path: "*.go"
`)
	if err != nil {
		t.Fatal(err)
	}
	want := "flowchart TD\n" +
		"  subgraph post_edit\n" +
		"    n0[\"when path=/polix.yaml<br/>run polix config fmt<br/>from guardrails.builtin.format_polix_files\"]\n" +
		"    n1[\"when path=ai/rules/*.yaml<br/>run polix rules fmt<br/>from guardrails.builtin.format_polix_files\"]\n" +
		"    n0 --> n1\n" +
		"    n2[\"when path=*.go<br/>run gofmt -w {path}<br/>from guardrails.format_on_edit\"]\n" +
		"    n1 --> n2\n" +
		"  end\n"
	if out := Decide(wf, true); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
}

// CUJ-04, CUJ-06, AC-03, AC-04: the subgraph guardrails draws first the items of forbidden_commands the presets of
// builtin add, each with its pattern, its reason and its origin, those of forbid_commit_no_verify also without gates
// of git, then the other protections, and no label of its own for the presets; a preset whose switch is false draws
// nothing.
func TestGraphBuiltinBlocks(t *testing.T) {
	wf, err := parseWorkflow(t, `version: 1
guardrails:
  builtin: { format_polix_files: false, check_polix_files: false }
  protected_files: [.env]
  forbidden_commands:
    - pattern: "rm -rf /*"
`)
	if err != nil {
		t.Fatal(err)
	}
	const noVerify = "<br/>Never skip the git hooks: fix what they block and commit again.<br/>" +
		"from guardrails.builtin.forbid_commit_no_verify"
	const destructive = "<br/>from guardrails.builtin.forbid_git_destructive_commands"
	labels := []string{
		"forbidden_commands: git commit*--no-verify*" + noVerify,
		"forbidden_commands: git commit* -n *" + noVerify,
		"forbidden_commands: git commit* -n" + noVerify,
		"forbidden_commands: git push*--force*<br/>Never force a push: ask the user to run it if it is needed." +
			destructive,
		"forbidden_commands: git push* -f*<br/>Never force a push: ask the user to run it if it is needed." + destructive,
		"forbidden_commands: git reset*--hard*<br/>git reset --hard discards changes for good: ask the user to run it." +
			destructive,
		"forbidden_commands: git clean* -f*<br/>git clean -f deletes untracked files for good: ask the user to run it." +
			destructive,
		"forbidden_commands: git clean*--force*<br/>git clean -f deletes untracked files for good: ask the user to " +
			"run it." + destructive,
		"protected_files: .env",
		"forbidden_commands: rm -rf /*",
	}
	var b strings.Builder
	b.WriteString("flowchart TD\n  subgraph guardrails\n")
	for i, l := range labels {
		fmt.Fprintf(&b, "    n%d[\"%s\"]\n", i, l)
		if i > 0 {
			fmt.Fprintf(&b, "    n%d --> n%d\n", i-1, i)
		}
	}
	b.WriteString("  end\n")
	if out, want := Decide(wf, true), b.String(); out.Mermaid != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.Mermaid, want)
	}
	for src, first := range map[string]string{
		"  builtin: { format_polix_files: false, check_polix_files: false, forbid_commit_no_verify: false }\n": labels[3],
		"  builtin: { format_polix_files: false, forbid_git_destructive_commands: false }\n":                   labels[0],
	} {
		wf, err := parseWorkflow(t, "version: 1\nguardrails:\n"+src)
		if err != nil {
			t.Fatal(err)
		}
		out := Decide(wf, true).Mermaid
		if !strings.Contains(out, "subgraph guardrails\n    n0[\""+first+"\"]\n") || strings.Contains(out, "builtin forbid") {
			t.Errorf("%s: got:\n%s", src, out)
		}
	}
	if out := Decide(wf, true).Mermaid; strings.Count(out, "from guardrails.builtin.forbid_commit_no_verify") != 3 {
		t.Errorf("the items of forbid_commit_no_verify without gates of git: got:\n%s", out)
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
