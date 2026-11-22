package log

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	stateadapters "github.com/mlophez/polix-ai/internal/app/state/adapters"
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

// load returns the state the loader of the log (stateadapters.Log) reads from the lines, so the records have the
// shape and the raw line they have in polix log.
func load(t *testing.T, lines ...string) state.State {
	t.Helper()
	p := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded := (&stateadapters.Log{}).Load(context.Background(), p)
	if loaded.Err != nil {
		t.Fatal(loaded.Err)
	}
	return loaded.State
}

// one is the port of a log of one session with the lines.
func one(t *testing.T, lines ...string) fakePort {
	t.Helper()
	return fakePort{sessions: []state.State{load(t, lines...)}}
}

// row is a row of the provisional layout: the time, the event and the actor of a record, then what it is.
func row(at, event, actor, what string) string {
	return fmt.Sprintf("%s  %-13s %-9s %s\n", at, event, actor, what)
}

// The lines of a session: legacy lines of three kinds and an old one, and an event line with two actions and two
// observations.
var lines = []string{
	`{"at":"t0","kind":"fact","session":"s1","event":"stop","actor":"session","hash":"fedcba9876543210","fact":"reviewed"}`,
	`{"at":"t1","kind":"observed","session":"s1","event":"subagent_stop","actor":"polix:reviewer",` +
		`"key":"polix:reviewer","outcome":"finished","hash":"0123456789abcdef"}`,
	`{"at":"t2","kind":"decision","session":"s1","event":"pre_edit","actor":"builder","action":"block","line":4,` +
		`"outcome":"block","reason":"first\nsecond"}`,
	`{"at":"t3","session":"s1","event":"pre_commit","actor":"session","branch":"main","head":"c1","hash":"h",` +
		`"input":{"command":"git commit"},"actions":[{"line":5,"action":"run","outcome":"skip",` +
		`"reason":"just test already passed on this content"},{"line":6,"action":"block","outcome":"block",` +
		`"reason":"revisa\nahora"}],"observed":[{"line":5,"outcome":"passed","key":"just test","hash":"h"},` +
		`{"outcome":"snapshot","key":"toolu_1","hash":"h"}]}`,
}

// Each action and each observation of a record is one row under the header of its session, with the time, the event
// and the actor of its record: an action with its verb, its outcome and what it acted on or its reason, an
// observation with its outcome and key, a legacy decision and a legacy observed line the same, and a line of a kind
// polix no longer writes with its kind and its fact.
func TestShellPrintsText(t *testing.T) {
	st := load(t, lines...)
	st.Skipped = 1
	res, err := run(context.Background(), fakePort{sessions: []state.State{st}}, Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "== session s1 ==\n" +
		row("t0", "stop", "session", "fact reviewed") +
		row("t1", "subagent_stop", "polix:reviewer", "observed finished polix:reviewer") +
		row("t2", "pre_edit", "builder", "decision block: block first") +
		row("t3", "pre_commit", "session", "action run: skip just test already passed on this content") +
		row("t3", "pre_commit", "session", "action block: block revisa") +
		row("t3", "pre_commit", "session", "observed passed just test") +
		row("t3", "pre_commit", "session", "observed snapshot toolu_1")
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
	if want := "polix: 1 lines of the log are longer than 4194304 bytes and were skipped\n"; errb.String() != want {
		t.Fatalf("stderr %q", errb.String())
	}
}

// --json prints the line of each record as it is in the file, the keys of an old fact line and of an event line
// included, in file order.
func TestShellPrintsJSON(t *testing.T) {
	res, err := run(context.Background(), one(t, lines...), Request{Tail: 5, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, true)
	if want := strings.Join(lines, "\n") + "\n"; out.String() != want || errb.Len() != 0 {
		t.Fatalf("got %q %q", out.String(), errb.String())
	}
}

// AC7, AC9, AC10: the sessions are printed in the order of their last activity, each under its header in the text
// and only as lines with --json; the warning of the skipped lines sums every file.
func TestShellPrintsSessions(t *testing.T) {
	late := load(t, `{"at":"2026-11-12T10:00:00Z","kind":"decision","session":"late"}`)
	late.Skipped = 2
	early := load(t,
		`{"at":"2026-11-12T08:00:00Z","kind":"decision","session":"early","outcome":"a"}`,
		`{"at":"2026-11-12T09:00:00Z","kind":"decision","session":"early","outcome":"b"}`)
	early.Skipped = 1
	port := fakePort{sessions: []state.State{late, early}}
	res, err := run(context.Background(), port, Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "== session early ==\n" +
		row("2026-11-12T08:00:00Z", "", "", "decision : a") +
		row("2026-11-12T09:00:00Z", "", "", "decision : b") +
		"== session late ==\n" +
		row("2026-11-12T10:00:00Z", "", "", "decision :")
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

// With --json the line of an event line and the lines of the legacy ones are printed byte for byte as in the file, in
// file order, the keys this polix does not know and the spaces of the line included, and no header comes before.
func TestShellPrintsJSONLines(t *testing.T) {
	file := []string{
		`{"at":"t1","kind":"observed","session":"s1","branch":"main","hash":"h","head":"c1","key":"just test","outcome":"passed"}`,
		`{"at":"t2","kind":"decision","session":"s1","branch":"main","hash":"h","outcome":"skip"}`,
		`{"at":"2026-11-22T13:50:05+01:00","agent":"claude","session":"s1","event":"pre_commit","actor":"session",` +
			`"branch":"main","head":"c0","hash":"h","input":{"command":"git commit"},"actions":[{"line":4,` +
			`"action":"inject","outcome":"injected","key":"ai/rules/go.yaml"}],"observed":[{"line":5,"outcome":"passed",` +
			`"key":"just test","hash":"h"}],"newer":true}`,
		`{ "at": "t3", "session": "s1", "observed": [ { "outcome": "started" } ] }`,
	}
	repo := newRepo(t, map[string]string{sessions + "s1.jsonl": strings.Join(file, "\n") + "\nnot json\n"})
	code, out, errb := runCommand(t, repo, "log", "--session", "s1", "--json")
	if want := strings.Join(file, "\n") + "\n"; code != 0 || out != want || errb != "" {
		t.Fatalf("json: %d %q %q", code, out, errb)
	}
}

// With --json the HEAD commit of a record is printed as it is in the file, after the hash, and a record without it
// has none.
func TestShellPrintsJSONHead(t *testing.T) {
	port := one(t,
		`{"at":"t1","kind":"observed","session":"s","branch":"main","hash":"h","head":"c1","key":"just test","outcome":"passed"}`,
		`{"at":"t2","kind":"observed","session":"s","branch":"main","hash":"h","key":"just test","outcome":"passed"}`,
		`{"at":"t3","session":"s","branch":"main","head":"c2","hash":"h","observed":[{"outcome":"passed",`+
			`"key":"just test","hash":"h"}]}`,
	)
	res, err := run(context.Background(), port, Request{Tail: 5, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, true)
	want := `{"at":"t1","kind":"observed","session":"s","branch":"main","hash":"h","head":"c1","key":"just test",` +
		`"outcome":"passed"}` + "\n" +
		`{"at":"t2","kind":"observed","session":"s","branch":"main","hash":"h","key":"just test","outcome":"passed"}` + "\n" +
		`{"at":"t3","session":"s","branch":"main","head":"c2","hash":"h","observed":[{"outcome":"passed",` +
		`"key":"just test","hash":"h"}]}` + "\n"
	if out.String() != want || errb.Len() != 0 {
		t.Fatalf("got %q %q", out.String(), errb.String())
	}
	if h := port.sessions[0].Records[0].Head; h != "c1" || port.sessions[0].Records[1].Head != "" {
		t.Fatalf("heads %q %q", h, port.sessions[0].Records[1].Head)
	}
}

// The rows of the repeat rules: a verb spared as a skip action with its verb, a passed observation with what it ran,
// the old injected and stop_block lines with their kind, the rule of the injected one included, and an inject of a
// text of several lines with its first line only.
func TestShellPrintsRepeatLines(t *testing.T) {
	port := one(t,
		`{"at":"t3","session":"s","event":"stop","actor":"session","actions":[{"line":5,"action":"run","outcome":"skip",`+
			`"reason":"just test already passed on this content"}],"observed":[{"line":5,"outcome":"passed",`+
			`"key":"just test","hash":"0123456789abcdef"}]}`,
		`{"at":"t5","kind":"injected","session":"s","rule":"GO-2"}`,
		`{"at":"t6","kind":"stop_block","session":"s","event":"stop","actor":"session","line":4,`+
			`"hash":"abcdef0123456789","reason":"revisa"}`,
		`{"at":"t7","session":"s","event":"pre_edit","actor":"session","observed":[{"line":9,"outcome":"injected",`+
			`"key":"uno\ndos"}]}`,
	)
	res, err := run(context.Background(), port, Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	want := "== session s ==\n" +
		row("t3", "stop", "session", "action run: skip just test already passed on this content") +
		row("t3", "stop", "session", "observed passed just test") +
		row("t5", "", "", "injected GO-2") +
		row("t6", "stop", "session", "stop_block revisa") +
		row("t7", "pre_edit", "session", "observed injected uno")
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
