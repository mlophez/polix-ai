package log

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/state"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// madrid sets the local time zone to the one of Europe/Madrid in November (UTC+1) for the test, so TIME shows the
// same on every machine.
func madrid(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("CET", 3600)
	t.Cleanup(func() { time.Local = old })
}

// plain forces the output of the command to plain text, as on a pipe or in CI.
func plain(t *testing.T) {
	t.Helper()
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "1")
}

// table prints the records of the lines as polix log prints them to a buffer, without colors, and returns stdout and
// stderr.
func table(t *testing.T, lines ...string) (stdout, stderr string) {
	t.Helper()
	madrid(t)
	plain(t)
	res, err := run(context.Background(), one(t, lines...), Request{Tail: 5})
	if err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	res.print(&out, &errb, false)
	return out.String(), errb.String()
}

// The event line of CUJ-06: an inject, a run that passed and took 8400 ms, a block, and the observation of the run.
const commitLine = `{"at":"2026-11-22T13:50:05+01:00","agent":"claude","session":"4037ea3d-b191-4a27-970d-92ce6488cd63",` +
	`"event":"pre_commit","actor":"session","branch":"main","head":"c0","hash":"h",` +
	`"input":{"command":"git commit -m \"wip\"","tool_use":"toolu_1"},"actions":[` +
	`{"line":4,"action":"inject","outcome":"injected","key":"ai/rules/go.yaml"},` +
	`{"line":5,"action":"run","outcome":"passed","key":"just test","duration_ms":8400},` +
	`{"line":6,"action":"block","outcome":"block","reason":"Subagents never commit."}],` +
	`"observed":[{"line":5,"outcome":"passed","key":"just test","hash":"h"}]}`

// tableOf joins the lines of a table, each ended by a line break.
func tableOf(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

// CUJ-14, LIT-07: the table has a header row and one block per event, each action on a line of its own under
// ACTIONS, the actions aligned, the local time, the session cut to 8 characters, and the observation of the run left
// out.
func TestShellPrintsTable(t *testing.T) {
	out, errb := table(t, commitLine)
	gap := strings.Repeat(" ", 52)
	want := tableOf(
		"TIME                 SESSION   EVENT       ACTOR    ACTIONS                         OBSERVED",
		"2026-11-22 13:50:05  4037ea3d  pre_commit  session  inject: ai/rules/go.yaml",
		gap+"run: passed, just test, 8.4s",
		gap+"block: Subagents never commit.",
	)
	if out != want || errb != "" {
		t.Fatalf("got:\n%s\nwant:\n%s\nstderr %q", out, want, errb)
	}
}

// CUJ-15, LIT-08: a start with a run, a post_command with only a snapshot and a watched, and a subagent_stop with a
// finished: the observation of the run and the post_command are left out.
func TestShellHidesRows(t *testing.T) {
	out, _ := table(t,
		`{"at":"2026-11-22T13:49:49+01:00","session":"4037ea3d-b191-4a27-970d-92ce6488cd63","event":"start","actor":"session",`+
			`"actions":[{"line":3,"action":"run","outcome":"passed","key":"polix install --githooks","duration_ms":300}],`+
			`"observed":[{"line":3,"outcome":"passed","key":"polix install --githooks","hash":"h"},`+
			`{"outcome":"started","key":"startup"}]}`,
		`{"at":"2026-11-22T13:50:00+01:00","session":"4037ea3d-b191-4a27-970d-92ce6488cd63","event":"post_command",`+
			`"actor":"session","observed":[{"outcome":"snapshot","key":"toolu_1","hash":"h"},`+
			`{"outcome":"watched","key":"toolu_1"}]}`,
		`{"at":"2026-11-22T13:51:00+01:00","session":"4037ea3d-b191-4a27-970d-92ce6488cd63","event":"subagent_stop",`+
			`"actor":"polix:reviewer","observed":[{"outcome":"finished","key":"polix:reviewer"}]}`,
	)
	want := tableOf(
		"TIME                 SESSION   EVENT          ACTOR           ACTIONS                                       OBSERVED",
		"2026-11-22 13:49:49  4037ea3d  start          session         run: passed, polix install --githooks, 300ms  started: startup",
		"2026-11-22 13:51:00  4037ea3d  subagent_stop  polix:reviewer                                                finished: polix:reviewer",
	)
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

// Each observation that repeats an action of its event is left out: injected after an inject, blocked after a block or
// a run that blocked, and notified after a notify; one with no action to repeat it stays.
func TestShellHidesRepeatedObservations(t *testing.T) {
	out, _ := table(t,
		`{"at":"2026-11-22T13:50:00+01:00","session":"s1","event":"stop","actor":"session","actions":[`+
			`{"line":1,"action":"inject","outcome":"injected","key":"a.md"},`+
			`{"line":2,"action":"block","outcome":"block","reason":"no"},`+
			`{"line":3,"action":"notify","outcome":"notify","reason":"hi"}],"observed":[`+
			`{"line":1,"outcome":"injected","key":"a.md"},{"line":2,"outcome":"blocked","hash":"h"},`+
			`{"line":3,"outcome":"notified","key":"hi"}]}`,
		`{"at":"2026-11-22T13:50:01+01:00","session":"s1","event":"stop","actor":"session","actions":[`+
			`{"line":4,"action":"run","outcome":"block","key":"just test","duration_ms":0,"reason":"fail"}],`+
			`"observed":[{"line":4,"outcome":"blocked","hash":"h"}]}`,
		`{"at":"2026-11-22T13:50:02+01:00","session":"s1","event":"pre_edit","actor":"session","observed":[`+
			`{"line":9,"outcome":"injected","key":"b.md"}]}`,
	)
	gap := strings.Repeat(" ", 49)
	want := tableOf(
		"TIME                 SESSION  EVENT     ACTOR    ACTIONS                     OBSERVED",
		"2026-11-22 13:50:00  s1       stop      session  inject: a.md",
		gap+"block: no",
		gap+"notify: hi",
		"2026-11-22 13:50:01  s1       stop      session  run: block, just test, 0ms",
		"2026-11-22 13:50:02  s1       pre_edit  session                              injected: b.md",
	)
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}

// CUJ-12: a legacy decision, a legacy observed line and a legacy fact line each print as a block of their own, the
// decision and the fact in ACTIONS and the observed line in OBSERVED.
func TestShellPrintsLegacyRows(t *testing.T) {
	out, errb := table(t,
		`{"at":"2026-11-22T13:50:00+01:00","kind":"decision","session":"s1","event":"stop","actor":"session",`+
			`"action":"block","line":4,"outcome":"block","reason":"revisa\nahora"}`,
		`{"at":"2026-11-22T13:50:01+01:00","kind":"observed","session":"s1","event":"subagent_stop",`+
			`"actor":"polix:reviewer","key":"polix:reviewer","outcome":"finished","hash":"0123456789abcdef"}`,
		`{"at":"2026-11-22T13:50:02+01:00","kind":"fact","session":"s1","event":"stop","actor":"session","fact":"reviewed"}`,
	)
	want := tableOf(
		"TIME                 SESSION  EVENT          ACTOR           ACTIONS         OBSERVED",
		"2026-11-22 13:50:00  s1       stop           session         block: revisa",
		"2026-11-22 13:50:01  s1       subagent_stop  polix:reviewer                  finished: polix:reviewer",
		"2026-11-22 13:50:02  s1       stop           session         fact: reviewed",
	)
	if out != want || errb != "" {
		t.Fatalf("got:\n%s\nwant:\n%s\nstderr %q", out, want, errb)
	}
}

// AC-14: the output to a buffer holds no escape code, an action line over 44 characters is cut to 44 with three
// dots, and a duration is truncated: 8450 ms is 8.4s, 999 ms 999ms and 1000 ms 1.0s.
func TestShellPlainWithoutTerminal(t *testing.T) {
	long := strings.Repeat("x", 80)
	out, _ := table(t,
		`{"at":"2026-11-22T13:50:00+01:00","session":"s1","event":"stop","actor":"session","actions":[`+
			`{"line":1,"action":"run","outcome":"passed","key":"a","duration_ms":8450},`+
			`{"line":2,"action":"run","outcome":"passed","key":"b","duration_ms":999},`+
			`{"line":3,"action":"run","outcome":"passed","key":"c","duration_ms":1000},`+
			`{"line":4,"action":"block","outcome":"block","reason":"`+long+`"}]}`,
	)
	if strings.Contains(out, "\x1b") {
		t.Fatalf("escape code in %q", out)
	}
	for _, want := range []string{"run: passed, a, 8.4s", "run: passed, b, 999ms", "run: passed, c, 1.0s",
		"block: " + strings.Repeat("x", 34) + "..."} {
		if !strings.Contains(out, want+"\n") {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	for l := range strings.SplitSeq(out, "\n") {
		if cell := strings.TrimSpace(l[min(len(l), 40):]); strings.Contains(l, "xxx") && len(cell) > 44 {
			t.Fatalf("line over 44: %q", l)
		}
	}
}

// CUJ-16: without the sessions folder, without sessions and with only hidden events, the table and --json print
// nothing to stdout nor to stderr, and the exit is 0.
func TestShellPrintsNothingWithoutEvents(t *testing.T) {
	plain(t)
	hidden := `{"at":"2026-11-22T13:50:00+01:00","session":"s1","event":"post_command","actor":"session",` +
		`"observed":[{"outcome":"snapshot","key":"toolu_1","hash":"h"},{"outcome":"watched","key":"toolu_1"}]}` + "\n"
	without := newRepo(t, nil)
	empty := newRepo(t, nil)
	if err := os.MkdirAll(filepath.Join(empty.Root, filepath.FromSlash(sessions)), 0o755); err != nil {
		t.Fatal(err)
	}
	onlyHidden := newRepo(t, map[string]string{sessions + "s1.jsonl": hidden})
	for name, repo := range map[string]*git.Repo{"without": without, "empty": empty, "hidden": onlyHidden} {
		for _, args := range [][]string{{"log"}, {"log", "--json"}} {
			code, out, errb := runCommand(t, repo, args...)
			if code != 0 || out != "" || errb != "" {
				t.Fatalf("%s %v: exit %d, stdout %q, stderr %q", name, args, code, out, errb)
			}
		}
	}
}

// CUJ-17: a key with a line break and a tab prints on one line with single spaces, a reason loses its escape code, the
// session, the event and the actor lose theirs too, and an at that does not parse prints as it is.
func TestShellFlattensCells(t *testing.T) {
	out, _ := table(t,
		`{"at":"not a time","session":"s1","event":"pre_command","actor":"session","actions":[`+
			`{"line":1,"action":"run","outcome":"passed","key":"echo a\n\tb   c","duration_ms":1},`+
			`{"line":2,"action":"block","outcome":"block","reason":"\u001b[2Jred\u0007\nsecond"}]}`,
	)
	if strings.Contains(out, "\x1b") || strings.Contains(out, "\a") {
		t.Fatalf("control character in %q", out)
	}
	for _, line := range []string{"not a time  s1       pre_command  session  run: passed, echo a b c, 1ms\n",
		"block: [2Jred\n"} {
		if !strings.Contains(out, line) {
			t.Fatalf("missing %q in:\n%s", line, out)
		}
	}
	if n := strings.Count(out, "\n"); n != 3 {
		t.Fatalf("%d lines in:\n%s", n, out)
	}
}

// ansi matches the escape codes of colors.
var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// CUJ-17: with colors forced, the name of each action takes the color of its outcome and OBSERVED is dimmed; without
// the color codes the lines equal those of the plain output.
func TestShellColorsOnTerminal(t *testing.T) {
	madrid(t)
	file := map[string]string{sessions + "s1.jsonl": strings.Join([]string{
		`{"at":"2026-11-22T13:50:00+01:00","session":"s1","event":"stop","actor":"session","actions":[` +
			`{"line":1,"action":"block","outcome":"block","reason":"no"},` +
			`{"line":2,"action":"run","outcome":"passed","key":"just test","duration_ms":3},` +
			`{"line":3,"action":"run","outcome":"skip","reason":"already"},` +
			`{"line":4,"action":"inject","outcome":"injected","key":"a.md"},` +
			`{"line":5,"action":"notify","outcome":"notify","reason":"hi"}],` +
			`"observed":[{"outcome":"started","key":"startup"},{"outcome":"finished","key":"polix:reviewer"}]}`,
	}, "\n") + "\n"}
	repo := newRepo(t, file)
	plain(t)
	_, want, _ := runCommand(t, repo, "log")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")
	code, out, errb := runCommand(t, repo, "log")
	if code != 0 || errb != "" {
		t.Fatalf("exit %d, stderr %q", code, errb)
	}
	if got := ansi.ReplaceAllString(out, ""); got != want {
		t.Fatalf("without the codes:\n%q\nplain:\n%q", got, want)
	}
	for _, want := range []string{"\x1b[1;31mblock", "\x1b[32mrun", "\x1b[90mrun", "\x1b[32minject", "\x1b[32mnotify",
		"\x1b[90mstarted: startup", "\x1b[90mfinished: polix:reviewer"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
}

// CUJ-17: a line over state.MaxLine is skipped with the warning on stderr, and the table goes to stdout.
func TestShellWarnsSkippedLines(t *testing.T) {
	plain(t)
	madrid(t)
	file := `{"session":"s1","reason":"` + strings.Repeat("x", state.MaxLine+1) + "\"}\n" +
		`{"at":"2026-11-22T13:50:00+01:00","session":"s1","event":"stop","actor":"session","actions":[` +
		`{"line":1,"action":"block","outcome":"block","reason":"no"}]}` + "\n"
	repo := newRepo(t, map[string]string{sessions + "s1.jsonl": file})
	code, out, errb := runCommand(t, repo, "log")
	if code != 0 || !strings.HasPrefix(errb, "polix: 1 lines of the log are longer than ") ||
		!strings.Contains(out, "block: no\n") || strings.Contains(out, "polix:") {
		t.Fatalf("exit %d\nstdout %q\nstderr %q", code, out, errb)
	}
}

// A line of an old kind prints as its kind and its fact, its rule, or the first line of its key or reason, in
// ACTIONS.
func TestShellPrintsOldKinds(t *testing.T) {
	out, _ := table(t,
		`{"at":"2026-11-22T13:50:00+01:00","kind":"injected","session":"s1","rule":"GO-2"}`,
		`{"at":"2026-11-22T13:50:01+01:00","kind":"stop_block","session":"s1","event":"stop","actor":"session",`+
			`"line":4,"hash":"abcdef0123456789","reason":"revisa\nahora"}`,
	)
	want := tableOf(
		"TIME                 SESSION  EVENT  ACTOR    ACTIONS             OBSERVED",
		"2026-11-22 13:50:00  s1                       injected: GO-2",
		"2026-11-22 13:50:01  s1       stop   session  stop_block: revisa",
	)
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
}
