package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/state"
)

// logWorkflowFile is the log of a repository, relative to its root.
const logWorkflowFile = ".git/" + state.DirName + "/" + state.LogName

func TestLogWorkflowAdapterReadLog(t *testing.T) {
	ctx := context.Background()
	st, err := LogWorkflowAdapter{Repo: workflowTestRepo(t, nil)}.ReadLog(ctx)
	if err != nil || len(st.Entries) != 0 || st.Skipped != 0 {
		t.Fatalf("without log: %+v %v", st, err)
	}
	log := `{"at":"t1","kind":"fact","fact":"reviewed"}` + "\nnot json\n" +
		`{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + "\"}\n" +
		`{"at":"t2","kind":"stop_block"}` + "\n"
	st, err = LogWorkflowAdapter{Repo: workflowTestRepo(t, map[string]string{logWorkflowFile: log})}.ReadLog(ctx)
	if err != nil || len(st.Entries) != 2 || st.Skipped != 1 || st.Entries[1].At != "t2" ||
		st.Entries[0].Fact != "reviewed" {
		t.Fatalf("log: %+v %v", st, err)
	}
}

// A log with lines too long to read: polix log warns of the skipped lines and
// prints the rest, as text or JSON lines.
func TestLogWorkflowCommand(t *testing.T) {
	log := `{"at":"t1","kind":"stop_block","reason":"loop_guard"}` + "\n" +
		`{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + "\"}\n"
	repo := workflowTestRepo(t, map[string]string{logWorkflowFile: log})
	code, stdout, stderr := workflowTestRun(t, repo, "log", "-n", "0")
	if code != 0 || !strings.Contains(stdout, "loop_guard") || !strings.Contains(stderr, "1 lines") {
		t.Fatalf("log: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, _ = workflowTestRun(t, repo, "log", "--json")
	if want := `{"at":"t1","kind":"stop_block","reason":"loop_guard"}` + "\n"; code != 0 || stdout != want {
		t.Fatalf("json: %d %q", code, stdout)
	}
}
