package workflow

import (
	"strings"
	"testing"
)

// base_branch takes one branch or a list, and main is the default.
func TestBaseBranches(t *testing.T) {
	for src, want := range map[string]string{
		"version: 1\n":                           "main",
		"version: 1\nbase_branch: dev\n":         "dev",
		"version: 1\nbase_branch: [dev, main]\n": "dev,main",
	} {
		w, err := Parse([]byte(src), "dev")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(w.BaseBranches(), ","); got != want {
			t.Errorf("%q: BaseBranches = %q, want %q", src, got, want)
		}
	}
	w, err := Parse([]byte("version: 1\nguardrails:\n  protected_files: [.env, \"ai/rules/**\"]\n"), "dev")
	if err != nil || len(w.ProtectedFiles) != 2 || len(w.On) != 0 {
		t.Fatalf("protected_files: %v %+v", err, w)
	}
}

func TestBases(t *testing.T) {
	if got := Bases("dev", []string{"main"}); strings.Join(got, ",") != "dev" {
		t.Fatal(got)
	}
	if got := Bases("", []string{"main", "next"}); strings.Join(got, ",") != "main,next" {
		t.Fatal(got)
	}
}
