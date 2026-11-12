package log

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	stateload "github.com/mlophez/polix-ai/internal/app/state/load"
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
	root.AddCommand(Command(stateload.Command()))
	var out, errb bytes.Buffer
	code = cli.Run(root, append(args, "--dir", repo.Root), strings.NewReader(""), &out, &errb)
	return code, out.String(), errb.String()
}

// sessions is the folder of the files of the sessions of a repository, relative to its root.
const sessions = ".git/polix/sessions/"

// AC6: ReadLog reads each .jsonl file of the sessions folder, through the loader, and nothing else: neither the
// other files of the folder nor the old log.jsonl. Without the folder there are no sessions.
func TestAdapterReadLog(t *testing.T) {
	ctx := context.Background()
	sts, err := Adapter{Repo: newRepo(t, nil), State: stateload.Command()}.ReadLog(ctx)
	if err != nil || len(sts) != 0 {
		t.Fatalf("without sessions: %+v %v", sts, err)
	}
	s1 := `{"at":"t1","kind":"fact","session":"s1","fact":"reviewed"}` + "\nnot json\n" +
		`{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + "\"}\n" +
		`{"at":"t2","kind":"stop_block","session":"s1"}` + "\n"
	repo := newRepo(t, map[string]string{
		sessions + "s1.jsonl":  s1,
		sessions + "s2.jsonl":  `{"at":"t3","kind":"decision","session":"s2"}` + "\n",
		sessions + "notes.txt": `{"at":"t4","kind":"decision","session":"notes"}` + "\n",
		".git/polix/log.jsonl": `{"at":"t5","kind":"decision","session":"old"}` + "\n",
	})
	sts, err = Adapter{Repo: repo, State: stateload.Command()}.ReadLog(ctx)
	if err != nil || len(sts) != 2 {
		t.Fatalf("sessions: %+v %v", sts, err)
	}
	slices.SortFunc(sts, func(a, b state.State) int { return strings.Compare(a.Entries[0].At, b.Entries[0].At) })
	if st := sts[0]; len(st.Entries) != 2 || st.Skipped != 1 || st.Entries[1].At != "t2" ||
		st.Entries[0].Fact != "reviewed" {
		t.Fatalf("s1: %+v", st)
	}
	if st := sts[1]; len(st.Entries) != 1 || st.Entries[0].Session != "s2" {
		t.Fatalf("s2: %+v", st)
	}
}

// A file of a session that cannot be read is the error of ReadLog.
func TestAdapterReadLogUnreadable(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file modes do not deny access here")
	}
	repo := newRepo(t, map[string]string{sessions + "s1.jsonl": `{"at":"t1","kind":"decision","session":"s1"}` + "\n"})
	p := filepath.Join(repo.GitDir, "polix", "sessions", "s1.jsonl")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if _, err := (Adapter{Repo: repo, State: stateload.Command()}).ReadLog(context.Background()); err == nil {
		t.Fatal("want the error of the unreadable file")
	}
}

// A log with lines too long to read: polix log warns of the skipped lines and
// prints the rest, as text or JSON lines.
func TestCommand(t *testing.T) {
	log := `{"at":"t1","kind":"stop_block","session":"s1","reason":"loop_guard"}` + "\n" +
		`{"kind":"decision","reason":"` + strings.Repeat("x", 5*1024*1024) + "\"}\n"
	repo := newRepo(t, map[string]string{sessions + "s1.jsonl": log})
	code, stdout, stderr := runCommand(t, repo, "log", "-n", "0")
	if code != 0 || !strings.Contains(stdout, "loop_guard") || !strings.Contains(stderr, "1 lines") {
		t.Fatalf("log: %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	code, stdout, _ = runCommand(t, repo, "log", "--json")
	if want := `{"at":"t1","kind":"stop_block","session":"s1","reason":"loop_guard"}` + "\n"; code != 0 || stdout != want {
		t.Fatalf("json: %d %q", code, stdout)
	}
}

// AC7 to AC10: polix log shows the last 5 sessions by default, the most recently active last, each whole under its
// header; -n 1 the last one, -n 0 all of them, and --json only their lines. Without sessions it prints nothing.
func TestCommandSessions(t *testing.T) {
	code, stdout, stderr := runCommand(t, newRepo(t, nil), "log")
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("without sessions: %d %q %q", code, stdout, stderr)
	}
	files := map[string]string{}
	for i := range 7 {
		id := fmt.Sprintf("s%d", i)
		files[sessions+id+".jsonl"] = fmt.Sprintf(`{"at":"2026-11-12T0%d:00:00Z","kind":"decision","session":"%s"}`,
			i, id) + "\n" + fmt.Sprintf(`{"at":"2026-11-12T0%d:30:00Z","kind":"decision","session":"%s"}`, i, id) + "\n"
	}
	repo := newRepo(t, files)
	headers := func(out string) []string {
		var got []string
		for line := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(line, "== session ") {
				got = append(got, line)
			}
		}
		return got
	}
	code, stdout, _ = runCommand(t, repo, "log")
	want := []string{"== session s2 ==", "== session s3 ==", "== session s4 ==", "== session s5 ==",
		"== session s6 =="}
	if got := headers(stdout); code != 0 || !slices.Equal(got, want) || strings.Count(stdout, "\n") != 15 {
		t.Fatalf("default: %d %q", code, stdout)
	}
	code, stdout, _ = runCommand(t, repo, "log", "-n", "1")
	if got := headers(stdout); code != 0 || !slices.Equal(got, []string{"== session s6 =="}) ||
		strings.Count(stdout, "\n") != 3 {
		t.Fatalf("-n 1: %d %q", code, stdout)
	}
	code, stdout, _ = runCommand(t, repo, "log", "-n", "0")
	if got := headers(stdout); code != 0 || len(got) != 7 || got[0] != "== session s0 ==" {
		t.Fatalf("-n 0: %d %q", code, stdout)
	}
	code, stdout, _ = runCommand(t, repo, "log", "-n", "2", "--json")
	want = []string{`{"at":"2026-11-12T05:00:00Z","kind":"decision","session":"s5"}`,
		`{"at":"2026-11-12T05:30:00Z","kind":"decision","session":"s5"}`,
		`{"at":"2026-11-12T06:00:00Z","kind":"decision","session":"s6"}`,
		`{"at":"2026-11-12T06:30:00Z","kind":"decision","session":"s6"}`}
	if code != 0 || stdout != strings.Join(want, "\n")+"\n" {
		t.Fatalf("--json: %d %q", code, stdout)
	}
}
