package workflow

import (
	"strings"
	"testing"
)

// base_branch takes one branch or a list, and main is the default.
func TestBaseBranches(t *testing.T) {
	for want, base := range map[string][]string{
		"main":     nil,
		"dev":      {"dev"},
		"dev,main": {"dev", "main"},
	} {
		w := Workflow{Version: 1, Base: base}
		if got := strings.Join(w.BaseBranches(), ","); got != want {
			t.Errorf("%q: BaseBranches = %q, want %q", base, got, want)
		}
	}
}

func TestBases(t *testing.T) {
	if got := (Workflow{Base: []string{"main"}}).Bases("dev"); strings.Join(got, ",") != "dev" {
		t.Fatal(got)
	}
	if got := (Workflow{Base: []string{"main", "next"}}).Bases(""); strings.Join(got, ",") != "main,next" {
		t.Fatal(got)
	}
}
