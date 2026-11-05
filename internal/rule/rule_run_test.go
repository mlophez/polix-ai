package rule

import "testing"

func TestRunValid(t *testing.T) {
	if err := (Run{Rule: goRule, Argv: []string{"go", "test"}}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Run{Rule: goRule}).Valid(); err != nil {
		t.Fatalf("a run without argv runs the check as a program: %v", err)
	}
	if err := (Run{Rule: Rule{ID: "prose", Description: "no check"}}).Valid(); err == nil {
		t.Fatal("a rule without check has a run")
	}
	if err := (Run{Rule: Rule{Deterministic: true, Check: "c.sh"}}).Valid(); err == nil {
		t.Fatal("an invalid rule has a run")
	}
}
