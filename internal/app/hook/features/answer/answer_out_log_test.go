package answer

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/state"
	stateadapters "github.com/mlophez/polix-ai/internal/app/state/adapters"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// decision is a record with one action of the session s that blocks.
func decision(session, outcome string) state.Record {
	return state.Record{At: "t", Session: session, Actions: []state.Action{{Action: "block", Outcome: outcome}}}
}

// encodeLine caps the reason, so no line of the log comes near state.MaxLine, and the line decodes back.
func TestEncodeLineCapsReason(t *testing.T) {
	line, err := (&Adapter{}).encodeLine(state.Record{At: "2026-01-01T00:00:00Z", Session: "s",
		Actions: []state.Action{{Action: "block", Outcome: "block", Reason: strings.Repeat("é", 512*1024)}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(line) > state.MaxReason+512 || strings.Contains(string(line), "\n") {
		t.Fatalf("the line has %d bytes", len(line))
	}
	var l logLine
	if err := json.Unmarshal(line, &l); err != nil {
		t.Fatal(err)
	}
	if reason := l.Actions[0].Reason; len(reason) > state.MaxReason || !strings.HasSuffix(reason, "[truncated]") {
		t.Fatalf("reason of %d bytes", len(reason))
	}
}

// The event line has the keys of the log in order, the empty ones left out, and the duration of a run that ran, 0
// included: the shape of the line of a commit with an inject, a passing check and a block.
func TestAppendWritesOneEventLine(t *testing.T) {
	a := &Adapter{Repo: &git.Repo{GitDir: t.TempDir()}}
	zero, took := int64(0), int64(8400)
	r := state.Record{At: "2026-11-22T13:50:05+01:00", Agent: "claude", Session: "s1", Event: "pre_commit",
		Actor: "session", Branch: "main", Head: "c0ffee", Hash: "h",
		Input: state.Input{Command: `git commit -m "wip"`, ToolUse: "toolu_1"},
		Actions: []state.Action{
			{Line: 4, Action: "inject", Outcome: "injected", Key: "ai/rules/go.yaml"},
			{Line: 5, Action: "run", Outcome: "passed", Key: "just test", DurationMS: &took},
			{Line: 6, Action: "run", Outcome: "block", Key: "just lint", Reason: "failed", DurationMS: &zero},
			{Action: "builtin", Outcome: "deny", Reason: "no"}},
		Observations: []state.Observation{{Line: 5, Outcome: state.OutcomePassed, Key: "just test", Hash: "h"},
			{Outcome: state.OutcomeSnapshot, Key: "toolu_1", Hash: "h", Refs: "r"},
			{Line: 6, Outcome: state.OutcomeBlocked, Hash: "h", Before: "g"}}}
	if err := a.Append(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionLog(a.Repo.GitDir, "s1"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"2026-11-22T13:50:05+01:00","agent":"claude","session":"s1","event":"pre_commit","actor":"session",` +
		`"branch":"main","head":"c0ffee","hash":"h","input":{"command":"git commit -m \"wip\"","tool_use":"toolu_1"},` +
		`"actions":[{"line":4,"action":"inject","outcome":"injected","key":"ai/rules/go.yaml"},` +
		`{"line":5,"action":"run","outcome":"passed","key":"just test","duration_ms":8400},` +
		`{"line":6,"action":"run","outcome":"block","key":"just lint","reason":"failed","duration_ms":0},` +
		`{"action":"builtin","outcome":"deny","reason":"no"}],` +
		`"observed":[{"line":5,"outcome":"passed","key":"just test","hash":"h"},` +
		`{"outcome":"snapshot","key":"toolu_1","hash":"h","refs":"r"},{"line":6,"outcome":"blocked","hash":"h","before":"g"}]}` +
		"\n"
	if got := string(data); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	minimal, err := a.encodeLine(state.Record{At: "t", Session: "s", Observations: []state.Observation{
		{Outcome: state.OutcomeStarted}}})
	if err != nil || string(minimal) != `{"at":"t","session":"s","observed":[{"outcome":"started"}]}` {
		t.Fatalf("empty keys are left out: %s %v", minimal, err)
	}
	if _, err := a.encodeLine(state.Record{}); err == nil {
		t.Fatal("a record without session nor observations")
	}
	if _, err := a.encodeLine(state.Record{Session: "s", Legacy: "decision", Actions: []state.Action{{Action: "a",
		Outcome: "b"}}}); err == nil {
		t.Fatal("a legacy record is never written")
	}
}

// Each text of the input longer than 200 characters is cut to 200, the three dots included, on a character; one of
// 200 is written whole.
func TestEventLineShortensInput(t *testing.T) {
	a := &Adapter{}
	encode := func(in state.Input) logInput {
		line, err := a.encodeLine(state.Record{At: "t", Session: "s", Input: in, Actions: []state.Action{
			{Action: "block", Outcome: "block"}}})
		if err != nil {
			t.Fatal(err)
		}
		var l logLine
		if err := json.Unmarshal(line, &l); err != nil || l.Input == nil {
			t.Fatalf("%s %v", line, err)
		}
		return *l.Input
	}
	long := encode(state.Input{Command: strings.Repeat("a", 300), Prompt: strings.Repeat("é", 250), Path: "src/a.go"})
	if long.Command != strings.Repeat("a", 197)+"..." || len([]rune(long.Command)) != 200 ||
		long.Prompt != strings.Repeat("é", 197)+"..." || long.Path != "src/a.go" {
		t.Fatalf("long values: %+v", long)
	}
	if whole := encode(state.Input{Command: strings.Repeat("a", 200)}); whole.Command != strings.Repeat("a", 200) {
		t.Fatalf("a value of 200 characters: %q", whole.Command)
	}
	line, err := a.encodeLine(decision("s", "block"))
	if err != nil || strings.Contains(string(line), `"input"`) {
		t.Fatalf("an empty input is left out: %s %v", line, err)
	}
}

// sessionLog is the file of the log of session in the git directory gitDir.
func sessionLog(gitDir, session string) string {
	return filepath.Join(gitDir, "polix", "sessions", session+".jsonl")
}

// AC1: Append writes each record to the file of its own session, and nothing to the old log.jsonl.
func TestAppendToTheSessionFile(t *testing.T) {
	gitDir := t.TempDir()
	a := &Adapter{Repo: &git.Repo{GitDir: gitDir}}
	if err := a.Append(context.Background(), decision("s1", "skip")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionLog(gitDir, "s1"))
	want := `{"at":"t","session":"s1","actions":[{"action":"block","outcome":"skip"}]}` + "\n"
	if err != nil || string(data) != want {
		t.Fatalf("the file of s1: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "polix", "log.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("log.jsonl: %v", err)
	}
	if err := a.Append(context.Background(), decision("", "skip")); err == nil {
		t.Fatal("a record without a session is written")
	}
}

// AC2, AC5: LoadState reads only the file of the session it is given, in file order; a session without lines is
// empty, and the empty session is an error with an empty state.
func TestLoadStateOfTheSession(t *testing.T) {
	gitDir := t.TempDir()
	a := &Adapter{Repo: &git.Repo{GitDir: gitDir}, State: (&stateadapters.Log{}).Load}
	ctx := context.Background()
	for _, r := range []state.Record{decision("s1", "a"), decision("s2", "b"), decision("s1", "c")} {
		if err := a.Append(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	s, err := a.LoadState(ctx, "s1")
	if err != nil || len(s.Records) != 2 || s.Records[0].Actions[0].Outcome != "a" ||
		s.Records[1].Actions[0].Outcome != "c" {
		t.Fatalf("s1: %+v %v", s, err)
	}
	if s, err := a.LoadState(ctx, "s3"); err != nil || len(s.Records) != 0 {
		t.Fatalf("s3: %+v %v", s, err)
	}
	if s, err := a.LoadState(ctx, ""); err == nil || len(s.Records) != 0 || s.Skipped != 0 {
		t.Fatalf("no session: %+v %v", s, err)
	}
}

// AC6: the old log.jsonl is never read, and appends leave it on disk as it was.
func TestOldLogIgnored(t *testing.T) {
	gitDir := t.TempDir()
	old := filepath.Join(gitDir, "polix", "log.jsonl")
	const line = `{"at":"t0","kind":"decision","session":"s1","outcome":"old"}` + "\n"
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &Adapter{Repo: &git.Repo{GitDir: gitDir}, State: (&stateadapters.Log{}).Load}
	ctx := context.Background()
	if err := a.Append(ctx, decision("s1", "new")); err != nil {
		t.Fatal(err)
	}
	s, err := a.LoadState(ctx, "s1")
	if err != nil || len(s.Records) != 1 || s.Records[0].Actions[0].Outcome != "new" {
		t.Fatalf("s1: %+v %v", s, err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != line {
		t.Fatalf("log.jsonl: %q %v", data, err)
	}
}
