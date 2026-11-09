package log

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/cli"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// The adapter of LogWorkflow is tested against a temporary repository.

// newRepo creates a git repository with the files and opens it.
func newRepo(t *testing.T, files map[string]string) *git.Repo {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", "--initial-branch=main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	for f, data := range files {
		p := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	repo, err := git.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

// runCommand runs the command of LogWorkflow on the repository through internal/platform/cli, as polix does.
func runCommand(t *testing.T, repo *git.Repo, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	root := cli.Root("polix", "", "", "")
	root.AddCommand(Command())
	var out, errb bytes.Buffer
	code = cli.Run(root, append(args, "--dir", repo.Root), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

// file is the log of a repository, relative to its root.
const file = ".git/" + state.DirName + "/" + state.LogName

func TestAdapterReadLog(t *testing.T) {
	ctx := context.Background()
	st, err := Adapter{Repo: newRepo(t, nil)}.ReadLog(ctx)
	if err != nil || len(st.Entries) != 0 || st.Skipped != 0 {
		t.Fatalf("without log: %+v %v", st, err)
	}
	log := `{"at":"t1","kind":"fact","fact":"reviewed"}` + "\nnot json\n" +
		`{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + "\"}\n" +
		`{"at":"t2","kind":"stop_block"}` + "\n"
	st, err = Adapter{Repo: newRepo(t, map[string]string{file: log})}.ReadLog(ctx)
	if err != nil || len(st.Entries) != 2 || st.Skipped != 1 || st.Entries[1].At != "t2" ||
		st.Entries[0].Fact != "reviewed" {
		t.Fatalf("log: %+v %v", st, err)
	}
}

// A log with lines too long to read: polix log warns of the skipped lines and
// prints the rest, as text or JSON lines.
func TestCommand(t *testing.T) {
	log := `{"at":"t1","kind":"stop_block","reason":"loop_guard"}` + "\n" +
		`{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + "\"}\n"
	repo := newRepo(t, map[string]string{file: log})
	code, stdout, stderr := runCommand(t, repo, "log", "-n", "0")
	if code != 0 || !strings.Contains(stdout, "loop_guard") || !strings.Contains(stderr, "1 lines") {
		t.Fatalf("log: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, _ = runCommand(t, repo, "log", "--json")
	if want := `{"at":"t1","kind":"stop_block","reason":"loop_guard"}` + "\n"; code != 0 || stdout != want {
		t.Fatalf("json: %d %q", code, stdout)
	}
}
