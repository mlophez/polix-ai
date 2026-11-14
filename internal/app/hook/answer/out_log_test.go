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
	stateload "github.com/mlophez/polix-ai/internal/app/state/load"
	"github.com/mlophez/polix-ai/internal/platform/git"
)

// encodeLine caps the reason, so no line of the log comes near state.MaxLine, and the line decodes back.
func TestEncodeLineCapsReason(t *testing.T) {
	line, err := (&Adapter{}).encodeLine(state.Entry{At: "2026-01-01T00:00:00Z", Kind: state.KindDecision, Session: "s",
		Reason: strings.Repeat("é", 512*1024)})
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
	if e := state.Entry(l); len(e.Reason) > state.MaxReason || !strings.HasSuffix(e.Reason, "[truncated]") {
		t.Fatalf("reason of %d bytes", len(e.Reason))
	}
}

// The fields keep the names of the log, and empty ones are left out.
func TestEncodeLineFields(t *testing.T) {
	a := &Adapter{}
	line, err := a.encodeLine(state.Entry{At: "t", Kind: state.KindDecision, Session: "s", Line: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(line); got != `{"at":"t","kind":"decision","session":"s","line":3}` {
		t.Fatal(got)
	}
	line, err = a.encodeLine(state.Entry{At: "t", Kind: state.KindObserved, Session: "s", Actor: "polix:reviewer",
		Hash: "h", Key: "polix:reviewer", Outcome: state.OutcomeFinished})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"t","kind":"observed","session":"s","actor":"polix:reviewer","hash":"h","key":"polix:reviewer",` +
		`"outcome":"finished"}`
	if got := string(line); got != want {
		t.Fatal(got)
	}
	if _, err := a.encodeLine(state.Entry{}); err == nil {
		t.Fatal("an entry without kind")
	}
}

// AC6: Append writes the HEAD commit of an entry under the key head, right after the hash, and leaves it out when
// the entry has none.
func TestAppendHead(t *testing.T) {
	a := &Adapter{Repo: &git.Repo{GitDir: t.TempDir()}}
	ctx := context.Background()
	if err := a.Append(ctx, state.Entry{At: "t", Kind: state.KindObserved, Session: "s", Branch: "main", Hash: "h",
		Head: "c1", Key: "just test", Outcome: state.OutcomePassed}); err != nil {
		t.Fatal(err)
	}
	if err := a.Append(ctx, state.Entry{At: "t", Kind: state.KindDecision, Session: "s", Branch: "main", Hash: "h",
		Outcome: "skip"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionLog(a.Repo.GitDir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"at":"t","kind":"observed","session":"s","branch":"main","hash":"h","head":"c1","key":"just test",` +
		`"outcome":"passed"}` + "\n" + `{"at":"t","kind":"decision","session":"s","branch":"main","hash":"h","outcome":"skip"}` + "\n"
	if got := string(data); got != want {
		t.Fatal(got)
	}
}

// sessionLog is the file of the log of session in the git directory gitDir.
func sessionLog(gitDir, session string) string {
	return filepath.Join(gitDir, "polix", "sessions", session+".jsonl")
}

// AC1: Append writes each entry to the file of its own session, and nothing to the old log.jsonl.
func TestAppendToTheSessionFile(t *testing.T) {
	gitDir := t.TempDir()
	a := &Adapter{Repo: &git.Repo{GitDir: gitDir}}
	if err := a.Append(context.Background(), state.Entry{At: "t", Kind: state.KindDecision, Session: "s1",
		Outcome: "skip"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(sessionLog(gitDir, "s1"))
	if err != nil || string(data) != `{"at":"t","kind":"decision","session":"s1","outcome":"skip"}`+"\n" {
		t.Fatalf("the file of s1: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(gitDir, "polix", "log.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("log.jsonl: %v", err)
	}
}

// AC2, AC5: LoadState reads only the file of the session it is given, in file order; a session without lines is
// empty, and the empty session is an error with an empty state.
func TestLoadStateOfTheSession(t *testing.T) {
	gitDir := t.TempDir()
	a := &Adapter{Repo: &git.Repo{GitDir: gitDir}, State: stateload.Command()}
	ctx := context.Background()
	for _, e := range []state.Entry{
		{At: "t1", Kind: state.KindDecision, Session: "s1", Outcome: "a"},
		{At: "t2", Kind: state.KindDecision, Session: "s2", Outcome: "b"},
		{At: "t3", Kind: state.KindDecision, Session: "s1", Outcome: "c"},
	} {
		if err := a.Append(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	s, err := a.LoadState(ctx, "s1")
	if err != nil || len(s.Entries) != 2 || s.Entries[0].Outcome != "a" || s.Entries[1].Outcome != "c" {
		t.Fatalf("s1: %+v %v", s, err)
	}
	if s, err := a.LoadState(ctx, "s3"); err != nil || len(s.Entries) != 0 {
		t.Fatalf("s3: %+v %v", s, err)
	}
	if s, err := a.LoadState(ctx, ""); err == nil || len(s.Entries) != 0 || s.Skipped != 0 {
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
	a := &Adapter{Repo: &git.Repo{GitDir: gitDir}, State: stateload.Command()}
	ctx := context.Background()
	if err := a.Append(ctx, state.Entry{At: "t1", Kind: state.KindDecision, Session: "s1", Outcome: "new"}); err != nil {
		t.Fatal(err)
	}
	s, err := a.LoadState(ctx, "s1")
	if err != nil || len(s.Entries) != 1 || s.Entries[0].Outcome != "new" {
		t.Fatalf("s1: %+v %v", s, err)
	}
	if data, err := os.ReadFile(old); err != nil || string(data) != line {
		t.Fatalf("log.jsonl: %q %v", data, err)
	}
}
