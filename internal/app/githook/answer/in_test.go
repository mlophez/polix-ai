package answer

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fake is the port of the shell without git nor processes: a repository with its workflow, branch and staged
// files, and the runs it carries out, each failing with the error of its command line.
type fake struct {
	outside   bool
	wf        workflow.Workflow
	present   bool
	problem   error
	branch    string
	staged    []string
	stagedErr error
	fails     map[string]error // the error of each command line, joined with spaces; the others pass
	ran       []string         // the command lines run, in order
	asked     []string         // the facts read, in order
	message   string           // the content of the message file
	msgErr    error            // why the message file cannot be read
	comment   string           // the comment string of git
}

func (f *fake) Message(_ context.Context, file string) (string, error) {
	f.asked = append(f.asked, "message "+file)
	return f.message, f.msgErr
}

func (f *fake) CommentChar(_ context.Context) string {
	f.asked = append(f.asked, "comment")
	return f.comment
}

func (f *fake) Open(_ context.Context, _ string) error {
	f.asked = append(f.asked, "open")
	if f.outside {
		return errors.New("not inside a git repository")
	}
	return nil
}

func (f *fake) LoadWorkflow(_ context.Context, _ string) (workflow.Workflow, bool, error) {
	f.asked = append(f.asked, "workflow")
	return f.wf, f.present, f.problem
}

func (f *fake) Branch(_ context.Context) string {
	f.asked = append(f.asked, "branch")
	return f.branch
}

func (f *fake) Staged(_ context.Context) ([]string, error) {
	f.asked = append(f.asked, "staged")
	return f.staged, f.stagedErr
}

func (f *fake) Run(_ context.Context, argv []string) error {
	line := strings.Join(argv, " ")
	f.ran = append(f.ran, line)
	return f.fails[line]
}

// AC4, AC9: the commit goes on with exit 0 and nothing to print, or is aborted with exit 1 and its reasons.
func TestRunExitCodes(t *testing.T) {
	ctx := context.Background()
	f := &fake{present: true, branch: "main", wf: gates(workflow.Entry{When: workflow.Cond{"branch": "^main$"},
		Action: blocks("No se commitea en main")})}
	res, err := run(ctx, f, Request{Event: workflow.PreCommit})
	if err != nil || res.Code != 1 || !slices.Equal(res.Reasons, []string{"No se commitea en main"}) {
		t.Fatalf("on main: %+v %v", res, err)
	}
	if !slices.Equal(f.asked, []string{"open", "workflow", "branch", "staged"}) {
		t.Fatalf("asked: %q", f.asked)
	}
	f = &fake{present: true, branch: "feature", wf: f.wf}
	if res, err := run(ctx, f, Request{Event: workflow.PreCommit}); err != nil || res.Code != 0 || res.Reasons != nil {
		t.Fatalf("on feature: %+v %v", res, err)
	}
	f = &fake{}
	if res, err := run(ctx, f, Request{Event: workflow.PreCommit}); err != nil || res.Code != 0 || res.Reasons != nil {
		t.Fatalf("no polix.yaml: %+v %v", res, err)
	}
	f = &fake{present: true, problem: workflow.InvalidError{"polix.yaml:2: x"}}
	if res, err := run(ctx, f, Request{Event: workflow.CommitMsg}); err != nil || res.Code != 1 ||
		!slices.Equal(res.Reasons, []string{"polix.yaml:2: x"}) {
		t.Fatalf("unusable polix.yaml: %+v %v", res, err)
	}
}

// AC9: outside a repository the commit goes on without reading anything else; an unknown event and staged files
// git cannot list are errors.
func TestRunErrors(t *testing.T) {
	ctx := context.Background()
	f := &fake{outside: true}
	if res, err := run(ctx, f, Request{Event: workflow.PreCommit}); err != nil || res.Code != 0 ||
		!slices.Equal(f.asked, []string{"open"}) {
		t.Fatalf("outside: %+v %v %q", res, err, f.asked)
	}
	f = &fake{}
	if _, err := run(ctx, f, Request{Event: "pre_push"}); err == nil || len(f.asked) != 0 {
		t.Fatalf("unknown event: %v %q", err, f.asked)
	}
	f = &fake{present: true, stagedErr: errors.New("git diff: boom")}
	if _, err := run(ctx, f, Request{Event: workflow.PreCommit}); err == nil {
		t.Fatal("staged files git cannot list are an error")
	}
}

// AC7: on commit_msg the shell reads the message file of the request and the comment string through the port, and
// the logic matches the first line of the message; a file that cannot be read is an error. pre_commit reads neither.
func TestRunCommitMsg(t *testing.T) {
	ctx := context.Background()
	wf := gates()
	wf.Githooks = map[string][]workflow.Entry{workflow.CommitMsg: {{Unless: workflow.Cond{"message": "^fix: "},
		Action: blocks("Empieza por fix: {message}")}}}
	f := &fake{present: true, wf: wf, branch: "main", message: "; nota\narregla\n", comment: ";"}
	res, err := run(ctx, f, Request{Event: workflow.CommitMsg, File: ".git/COMMIT_EDITMSG"})
	if err != nil || res.Code != 1 || !slices.Equal(res.Reasons, []string{"Empieza por fix: arregla"}) {
		t.Fatalf("got %+v %v", res, err)
	}
	if !slices.Equal(f.asked, []string{"open", "workflow", "branch", "staged", "message .git/COMMIT_EDITMSG",
		"comment"}) {
		t.Fatalf("asked: %q", f.asked)
	}
	f = &fake{present: true, wf: wf, message: "fix: x\n", comment: "#"}
	if res, err := run(ctx, f, Request{Event: workflow.CommitMsg, File: "msg"}); err != nil || res.Code != 0 {
		t.Fatalf("a message with fix: %+v %v", res, err)
	}
	f = &fake{present: true, wf: wf, msgErr: errors.New("open msg: no such file")}
	if _, err := run(ctx, f, Request{Event: workflow.CommitMsg, File: "msg"}); err == nil {
		t.Fatal("a message file that cannot be read is an error")
	}
	f = &fake{present: true, wf: wf}
	if _, err := run(ctx, f, Request{Event: workflow.PreCommit}); err != nil ||
		!slices.Equal(f.asked, []string{"open", "workflow", "branch", "staged"}) {
		t.Fatalf("pre_commit: %v %q", err, f.asked)
	}
}

// AC6: the shell carries out the runs the logic asks for, in order, and stops at the first that fails.
func TestRunCarriesOutRuns(t *testing.T) {
	f := &fake{present: true, branch: "main", fails: map[string]error{"just lint": errors.New("exit status 1")},
		wf: gates(workflow.Entry{Action: runs("", "just", "test")}, workflow.Entry{Action: runs("Lint", "just", "lint")},
			workflow.Entry{Action: runs("", "just", "never")})}
	res, err := run(context.Background(), f, Request{Event: workflow.PreCommit})
	if err != nil || res.Code != 1 || !slices.Equal(f.ran, []string{"just test", "just lint"}) ||
		!slices.Equal(res.Reasons, []string{"Lint", "just lint failed: exit status 1"}) {
		t.Fatalf("got %+v %v %q", res, err, f.ran)
	}
}

// Each line of each reason goes to stderr prefixed with polix:, without the last line break of a block message.
func TestResponsePrint(t *testing.T) {
	var stderr strings.Builder
	Response{Reasons: []string{"Uno\n\ndos\n", "just lint failed: exit status 1"}, Code: 1}.print(&stderr)
	if want := "polix: Uno\npolix:\npolix: dos\npolix: just lint failed: exit status 1\n"; stderr.String() != want {
		t.Fatalf("got %q, want %q", stderr.String(), want)
	}
}

// AC9: the command maps the events of git to those of githooks, an unknown one exits with 1, and outside a
// repository it exits with 0 and prints nothing; commit-msg needs the file of its message, which pre-commit does
// not take, and a file that cannot be read exits with 1.
func TestCommand(t *testing.T) {
	outside := t.TempDir()
	repo := testenv.GitRepo(t)
	for args, want := range map[string]int{
		"pre-commit": 0, "commit-msg msg": 0, "pre_commit": 1, "pre-push": 1, "commit-msg": 1, "pre-commit msg": 1,
	} {
		root := cli.Root("polix", "", "", "")
		root.AddCommand(Command(workflowload.Command()))
		var stdout, stderr strings.Builder
		argv := append(append([]string{"githook"}, strings.Fields(args)...), "--dir", outside)
		code := cli.Run(root, argv, strings.NewReader(""), &stdout, &stderr)
		if code != want || stdout.String() != "" || (want == 0) != (stderr.String() == "") {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", args, code, stdout.String(), stderr.String())
		}
	}
	root := cli.Root("polix", "", "", "")
	root.AddCommand(Command(workflowload.Command()))
	var stdout, stderr strings.Builder
	missing := filepath.Join(repo, "no-such-message")
	if code := cli.Run(root, []string{"githook", "commit-msg", missing, "--dir", repo}, strings.NewReader(""), &stdout,
		&stderr); code != 1 || !strings.HasPrefix(stderr.String(), "polix githook: ") {
		t.Fatalf("unreadable message: exit %d, stderr %q", code, stderr.String())
	}
}
