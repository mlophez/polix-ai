package rule

import (
	"errors"
	"strings"
	"testing"
)

var goRule = Rule{ID: "go", Description: "gofmt", Deterministic: true, Check: "ai/checks/gofmt.sh"}

func TestResultValid(t *testing.T) {
	if err := (Result{Rule: goRule}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Result{Rule: Rule{ID: "prose", Description: "no check"}}).Valid(); err == nil {
		t.Fatal("a rule without check has a result")
	}
	if err := (Result{Rule: Rule{ID: "x", Deterministic: true, Check: "c.sh"}}).Valid(); err == nil {
		t.Fatal("an invalid rule has a result")
	}
}

func TestOutcomeValid(t *testing.T) {
	if err := (Outcome{Output: "x", Err: errors.New("exit status 1")}).Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestResultTailKeepsTheLastLinesAndTheError(t *testing.T) {
	r := Result{Rule: goRule, Output: "a.go: not formatted\n\n", Err: errors.New("exit status 1")}
	if r.Passed() {
		t.Fatal("passed with an error")
	}
	if want := "a.go: not formatted\nexit status 1"; r.Tail() != want {
		t.Fatalf("tail %q, want %q", r.Tail(), want)
	}
}

// Tail keeps TailLines lines, the last ones, and cuts each to MaxLineLen bytes, so a huge output never reaches
// the console whole.
func TestResultTailBoundsTheOutput(t *testing.T) {
	var lines []string
	for i := range TailLines + 5 {
		lines = append(lines, strings.Repeat("x", i+1))
	}
	r := Result{Rule: goRule, Output: strings.Join(lines, "\n") + "\n" + strings.Repeat("y", 10000)}
	got := strings.Split(r.Tail(), "\n")
	if len(got) != TailLines || got[0] != lines[len(lines)-TailLines+1] {
		t.Fatalf("tail keeps %d lines from %q", len(got), got[0])
	}
	if last := got[len(got)-1]; len(last) > MaxLineLen || !strings.HasPrefix(last, "yyy") {
		t.Fatalf("the last line has %d bytes", len(last))
	}
}

func TestFailed(t *testing.T) {
	got := Results{{Rule: goRule}, {Rule: goRule, Err: errors.New("exit status 1")}}.Failed()
	if len(got) != 1 || got[0].Err == nil {
		t.Fatalf("got %+v", got)
	}
}
