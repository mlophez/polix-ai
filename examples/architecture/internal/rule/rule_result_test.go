package rule

import (
	"errors"
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

func TestFailed(t *testing.T) {
	got := Failed([]Result{{Rule: goRule}, {Rule: goRule, Err: errors.New("exit status 1")}})
	if len(got) != 1 || got[0].Err == nil {
		t.Fatalf("got %+v", got)
	}
}
