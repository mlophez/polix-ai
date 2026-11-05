package workflow

import (
	"slices"
	"strings"
	"testing"
)

// The texts of the switches turned on are returned commit first; none when every switch is off.
func TestGuidanceInstructions(t *testing.T) {
	cases := []struct {
		name string
		g    Guidance
		want []string
	}{
		{"both", Guidance{Commit: true, Edit: true}, []string{GuidanceCommit, GuidanceEdit}},
		{"only commit", Guidance{Commit: true}, []string{GuidanceCommit}},
		{"only edit", Guidance{Edit: true}, []string{GuidanceEdit}},
		{"none", Guidance{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.g.Instructions(); !slices.Equal(got, c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// An unknown key of guidance is a problem that names the keys allowed, also inside a workflow.
func TestGuidanceValid(t *testing.T) {
	g := Guidance{Commit: true, Keys: []string{"commit", "delegation"}}
	want := `unknown key "delegation" in guidance (commit, edit)`
	if err := g.Valid(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("guidance: %v", err)
	}
	if err := (Workflow{Version: 1, Guidance: g}).Valid(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("in a workflow: %v", err)
	}
	if err := (Guidance{Keys: []string{"commit", "edit"}}).Valid(); err != nil {
		t.Fatal(err)
	}
}
