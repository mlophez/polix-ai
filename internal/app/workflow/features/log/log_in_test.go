package log

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/git"
	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// fakePort answers the port of LogWorkflow from memory: the state of each file of a session.
type fakePort struct {
	sessions []state.State
	err      error
}

// TestMain runs the tests without the variables git exports to its hooks (see testenv.ClearGitHookEnv): the githooks
// of polix.yaml run go test, and these tests run git in repositories of their own.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

func (f fakePort) ReadLog(context.Context) ([]state.State, error) { return f.sessions, f.err }

var entries = []state.Entry{
	{At: "t0", Kind: "fact", Session: "s1", Event: "stop", Actor: "session", Hash: "fedcba9876543210", Fact: "reviewed"},
	{At: "t1", Kind: state.KindObserved, Session: "s1", Event: "subagent_stop", Actor: "polix:reviewer",
		Key: "polix:reviewer", Outcome: state.OutcomeFinished, Hash: "0123456789abcdef"},
	{At: "t2", Kind: state.KindDecision, Session: "s1", Event: "pre_edit", Actor: "builder", Action: "block",
		Outcome: "block", Line: 4, Reason: "first\nsecond"},
}

// one is the port of a log of one session with the entries.
func one(entries ...state.Entry) fakePort {
	return fakePort{sessions: []state.State{{Entries: entries}}}
}

// Each entry is one line under the header of its session: an observed one with its outcome and key, a decision
// with its outcome and verb, and an old line of a kind polix no longer writes with its kind.
func TestShellPrintsText(t *testing.T) {
	port := fakePort{sessions: []state.State{{Entries: entries, Skipped: 1}}}
	res, err := run(context.Background(), port, Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "== session s1 ==\n" +
		"t0  fact       stop          session   reviewed         fedcba98  \n" +
		"t1  observed   subagent_stop polix:reviewer finished polix:reviewer 01234567  \n" +
		"t2  decision   pre_edit      builder   block block        first\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
	if want := "polix: 1 lines of the log are longer than 4194304 bytes and were skipped\n"; errb.String() != want {
		t.Fatalf("stderr %q", errb.String())
	}
}

func TestShellPrintsJSON(t *testing.T) {
	res, err := run(context.Background(), one(entries...), Request{Tail: 5, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, true)
	// The old fact line keeps its name, read-only.
	want := `{"at":"t0","kind":"fact","session":"s1","event":"stop","actor":"session","hash":"fedcba9876543210",` +
		`"fact":"reviewed"}` + "\n" +
		`{"at":"t1","kind":"observed","session":"s1","event":"subagent_stop","actor":"polix:reviewer",` +
		`"hash":"0123456789abcdef","key":"polix:reviewer","outcome":"finished"}` + "\n" +
		`{"at":"t2","kind":"decision","session":"s1","event":"pre_edit","actor":"builder","action":"block","line":4,` +
		`"outcome":"block","reason":"first\nsecond"}` + "\n"
	if out.String() != want || errb.Len() != 0 {
		t.Fatalf("got %q %q", out.String(), errb.String())
	}
}

// AC7, AC9, AC10: the sessions are printed in the order of their last activity, each under its header in the text
// and only as lines with --json; the warning of the skipped lines sums every file.
func TestShellPrintsSessions(t *testing.T) {
	port := fakePort{sessions: []state.State{
		{Entries: []state.Entry{{At: "2026-11-12T10:00:00Z", Kind: state.KindDecision, Session: "late"}}, Skipped: 2},
		{Entries: []state.Entry{
			{At: "2026-11-12T08:00:00Z", Kind: state.KindDecision, Session: "early", Outcome: "a"},
			{At: "2026-11-12T09:00:00Z", Kind: state.KindDecision, Session: "early", Outcome: "b"},
		}, Skipped: 1},
	}}
	res, err := run(context.Background(), port, Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "== session early ==\n" +
		"2026-11-12T08:00:00Z  decision                           a                  \n" +
		"2026-11-12T09:00:00Z  decision                           b                  \n" +
		"== session late ==\n" +
		"2026-11-12T10:00:00Z  decision                                              \n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
	if want := "polix: 3 lines of the log are longer than 4194304 bytes and were skipped\n"; errb.String() != want {
		t.Fatalf("stderr %q", errb.String())
	}
	out.Reset()
	errb.Reset()
	res.print(&out, &errb, true)
	want = `{"at":"2026-11-12T08:00:00Z","kind":"decision","session":"early","outcome":"a"}` + "\n" +
		`{"at":"2026-11-12T09:00:00Z","kind":"decision","session":"early","outcome":"b"}` + "\n" +
		`{"at":"2026-11-12T10:00:00Z","kind":"decision","session":"late"}` + "\n"
	if out.String() != want {
		t.Fatalf("json:\n%q\nwant:\n%q", out.String(), want)
	}
	res, err = run(context.Background(), port, Request{Tail: 1})
	if err != nil || len(res.Sessions) != 1 || res.Sessions[0].ID != "late" || res.Skipped != 3 {
		t.Fatalf("tail 1: %+v %v", res, err)
	}
}

// AC-01: --tail counts sessions, 1 by default.
func TestCommandTailDefault(t *testing.T) {
	c := Command(nil)
	f := c.Flags().Lookup("tail")
	if f == nil || f.DefValue != "1" || f.Shorthand != "n" {
		t.Fatalf("flag tail: %+v", f)
	}
}

// threeSessions is a repository whose sessions folder holds three sessions, the most recently active first by name.
func threeSessions(t *testing.T) *git.Repo {
	t.Helper()
	line := func(id, at string) string {
		return `{"at":"` + at + `","kind":"decision","session":"` + id + `","outcome":"a"}` + "\n"
	}
	return newRepo(t, map[string]string{
		sessions + "a.jsonl": line("4037ea3d-b191-4a27-970d-92ce6488cd63", "2026-11-22T13:00:00Z"),
		sessions + "b.jsonl": line("bf6d55cb-3725-4ee0-aa97-551114a740f0", "2026-11-22T11:00:00Z"),
		sessions + "c.jsonl": line("8e90182c-cf0e-43ea-b3f1-f6f94bc6ad2f", "2026-11-22T12:00:00Z"),
	})
}

// shown returns the ids of the sessions the text output names, in order.
func shown(out string) []string {
	var ids []string
	for l := range strings.SplitSeq(out, "\n") {
		if id, ok := strings.CutPrefix(l, "== session "); ok {
			ids = append(ids, strings.TrimSuffix(id, " =="))
		}
	}
	return ids
}

// CUJ-01: without flags only the most recently active session is printed.
func TestShellShowsLastSessionByDefault(t *testing.T) {
	code, out, errb := runCommand(t, threeSessions(t), "log")
	if code != 0 || errb != "" {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if got, want := shown(out), []string{"4037ea3d-b191-4a27-970d-92ce6488cd63"}; !slices.Equal(got, want) {
		t.Fatalf("sessions %v:\n%s", got, out)
	}
}

// CUJ-02: -n 2 prints the two most recent sessions and --all the three, the most recent last; --all with -n fails.
func TestShellTailAndAll(t *testing.T) {
	repo := threeSessions(t)
	code, out, _ := runCommand(t, repo, "log", "-n", "2")
	want := []string{"8e90182c-cf0e-43ea-b3f1-f6f94bc6ad2f", "4037ea3d-b191-4a27-970d-92ce6488cd63"}
	if got := shown(out); code != 0 || !slices.Equal(got, want) {
		t.Fatalf("-n 2: exit %d, sessions %v", code, got)
	}
	code, out, _ = runCommand(t, repo, "log", "--all")
	want = []string{"bf6d55cb-3725-4ee0-aa97-551114a740f0", want[0], want[1]}
	if got := shown(out); code != 0 || !slices.Equal(got, want) {
		t.Fatalf("--all: exit %d, sessions %v", code, got)
	}
	code, out, errb := runCommand(t, repo, "log", "--all", "-n", "2")
	if code != 1 || out != "" || !strings.Contains(errb, "if any flags in the group [all tail] are set") {
		t.Fatalf("--all -n 2: exit %d, stdout %q, stderr %q", code, out, errb)
	}
}

// CUJ-03: a --tail below 1 fails with LIT-01 and prints nothing to stdout.
func TestCommandRejectsTailBelowOne(t *testing.T) {
	repo := threeSessions(t)
	for _, n := range []string{"0", "-1"} {
		code, out, errb := runCommand(t, repo, "log", "-n", n)
		if code != 1 || out != "" || !strings.Contains(errb, "--tail must be 1 or more; use --all for every session") {
			t.Fatalf("-n %s: exit %d, stdout %q, stderr %q", n, code, out, errb)
		}
	}
}

// CUJ-04: a prefix and a full id each print only their session, an older one included.
func TestShellSessionPrefix(t *testing.T) {
	repo := threeSessions(t)
	for arg, want := range map[string]string{"bf6d": "bf6d55cb-3725-4ee0-aa97-551114a740f0",
		"8e90182c-cf0e-43ea-b3f1-f6f94bc6ad2f": "8e90182c-cf0e-43ea-b3f1-f6f94bc6ad2f"} {
		code, out, errb := runCommand(t, repo, "log", "--session", arg)
		if got := shown(out); code != 0 || errb != "" || !slices.Equal(got, []string{want}) {
			t.Fatalf("--session %s: exit %d, sessions %v, stderr %q", arg, code, got, errb)
		}
	}
}

// CUJ-05: each --session that selects no single session, or comes with --tail or --all, fails and prints nothing.
func TestShellSessionErrors(t *testing.T) {
	repo := threeSessions(t)
	extra := `{"at":"2026-11-22T10:00:00Z","kind":"decision","session":"4a1b0c2d-0000-4000-8000-000000000000"}` + "\n"
	if err := os.WriteFile(filepath.Join(repo.Root, filepath.FromSlash(sessions+"d.jsonl")), []byte(extra), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no match", []string{"--session", "zz"}, `--session "zz" matches no session`},
		{"several", []string{"--session", "4"}, `--session "4" matches several sessions: ` +
			"4037ea3d-b191-4a27-970d-92ce6488cd63, 4a1b0c2d-0000-4000-8000-000000000000"},
		{"with tail", []string{"--session", "bf6d", "-n", "2"}, "--session selects one session: drop --tail and --all"},
		{"with all", []string{"--session", "bf6d", "--all"}, "--session selects one session: drop --tail and --all"},
		{"with both", []string{"--session", "bf6d", "--all", "-n", "2"},
			"--session selects one session: drop --tail and --all"},
		{"empty", []string{"--session", ""}, "--session needs a session id"},
	} {
		code, out, errb := runCommand(t, repo, append([]string{"log"}, tc.args...)...)
		if code != 1 || out != "" || !strings.Contains(errb, tc.want) {
			t.Fatalf("%s: exit %d, stdout %q, stderr %q", tc.name, code, out, errb)
		}
	}
}

// With --json the HEAD commit of an observed line is printed as head after the hash, and left out of a line
// without it.
func TestShellPrintsJSONHead(t *testing.T) {
	port := one(
		state.Entry{At: "t1", Kind: state.KindObserved, Session: "s", Branch: "main", Hash: "h", Head: "c1",
			Key: "just test", Outcome: state.OutcomePassed},
		state.Entry{At: "t2", Kind: state.KindObserved, Session: "s", Branch: "main", Hash: "h", Key: "just test",
			Outcome: state.OutcomePassed},
	)
	res, err := run(context.Background(), port, Request{Tail: 5, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, true)
	want := `{"at":"t1","kind":"observed","session":"s","branch":"main","hash":"h","head":"c1","key":"just test",` +
		`"outcome":"passed"}` + "\n" +
		`{"at":"t2","kind":"observed","session":"s","branch":"main","hash":"h","key":"just test","outcome":"passed"}` + "\n"
	if out.String() != want || errb.Len() != 0 {
		t.Fatalf("got %q %q", out.String(), errb.String())
	}
}

// The lines of the repeat rules: a verb spared as a skip decision with its verb, a passed line with what it ran, and
// the old injected and stop_block lines with their kind, the rule of the injected one included, and an inject of a
// text of several lines with its first line only.
func TestShellPrintsRepeatLines(t *testing.T) {
	port := one(
		state.Entry{At: "t3", Kind: state.KindDecision, Session: "s", Event: "stop", Actor: "session", Action: "run",
			Outcome: "skip", Line: 5, Reason: "just test already passed on this content"},
		state.Entry{At: "t4", Kind: state.KindObserved, Session: "s", Event: "stop", Actor: "session",
			Key: "just test", Outcome: state.OutcomePassed, Line: 5, Hash: "0123456789abcdef"},
		state.Entry{At: "t5", Kind: "injected", Session: "s", Rule: "GO-2"},
		state.Entry{At: "t6", Kind: "stop_block", Session: "s", Event: "stop", Actor: "session", Line: 4,
			Hash: "abcdef0123456789", Reason: "revisa"},
		state.Entry{At: "t7", Kind: state.KindObserved, Session: "s", Event: "pre_edit", Actor: "session",
			Key: "uno\ndos", Outcome: state.OutcomeInjected, Line: 9},
	)
	res, err := run(context.Background(), port, Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "== session s ==\n" +
		"t3  decision   stop          session   skip run           just test already passed on this content\n" +
		"t4  observed   stop          session   passed just test 01234567  \n" +
		"t5  injected                           GO-2               \n" +
		"t6  stop_block stop          session                    abcdef01  revisa\n" +
		"t7  observed   pre_edit      session   injected uno       \n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestShellError(t *testing.T) {
	boom := errors.New("boom")
	if _, err := run(context.Background(), fakePort{err: boom}, Request{}); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
}
