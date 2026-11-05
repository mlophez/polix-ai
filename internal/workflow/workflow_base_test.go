package workflow

import (
	"strings"
	"testing"
)

// base takes one branch or a list, and main is the default.
func TestBaseBranches(t *testing.T) {
	for src, want := range map[string]string{
		"version: 1\n":                    "main",
		"version: 1\nbase: dev\n":         "dev",
		"version: 1\nbase: [dev, main]\n": "dev,main",
	} {
		w, err := Parse([]byte(src), "dev")
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(w.BaseBranches(), ","); got != want {
			t.Errorf("%q: BaseBranches = %q, want %q", src, got, want)
		}
	}
	w, err := Parse([]byte("version: 1\nprotected: [.env, \"ai/rules/**\"]\n"), "dev")
	if err != nil || len(w.Protected) != 2 {
		t.Fatalf("protected: %v %+v", err, w)
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
