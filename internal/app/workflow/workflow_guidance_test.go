package workflow

import (
	"strings"
	"testing"
)

// The session context holds the header line and then the items of each switch turned on, in the order commit,
// edit, layout, with no blank lines; it is empty, header included, when every switch is off.
func TestWorkflowSessionContext(t *testing.T) {
	const header = "This project is managed by polix.\nFollow these rules:\n- "
	const commit, edit, layout = "- Run each git commit", "- Create and change files", "- Use these folders"
	all := []string{commit, edit, layout}
	cases := []struct {
		name string
		g    Guidance
		want []string
	}{
		{"all", Guidance{Commit: true, Edit: true, Layout: true}, all},
		{"only commit", Guidance{Commit: true}, []string{commit}},
		{"only edit", Guidance{Edit: true}, []string{edit}},
		{"only layout", Guidance{Layout: true}, []string{layout}},
		{"none", Guidance{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Workflow{Guidance: c.g}.SessionContext()
			if c.want == nil {
				if got != "" {
					t.Fatalf("got %q, want nothing", got)
				}
				return
			}
			if !strings.HasPrefix(got, header) || strings.Contains(got, "\n\n") || strings.TrimSpace(got) != got {
				t.Fatalf("want the header, then items with no blank lines: %q", got)
			}
			at := -1
			for _, item := range all {
				i := strings.Index(got, "\n"+item)
				if want := strings.Contains(strings.Join(c.want, "\x00"), item); (i >= 0) != want {
					t.Fatalf("item %q present %v, want %v: %q", item, i >= 0, want, got)
				}
				if i >= 0 && i < at {
					t.Fatalf("item %q out of order: %q", item, got)
				}
				at = max(at, i)
			}
		})
	}
}

// Every line of the session context is of at most 120 characters with the default folders, and after the header
// lines every line is an item.
func TestWorkflowSessionContextLines(t *testing.T) {
	got := Workflow{Guidance: Guidance{Commit: true, Edit: true, Layout: true}}.SessionContext()
	items := false
	for i, line := range strings.Split(got, "\n") {
		if n := len([]rune(line)); n > 120 {
			t.Errorf("line %d has %d characters: %q", i, n, line)
		}
		item := strings.HasPrefix(strings.TrimLeft(line, " "), "- ")
		if items && !item {
			t.Errorf("line %d is not an item: %q", i, line)
		}
		items = items || item
	}
	if !items {
		t.Fatalf("no items: %q", got)
	}
}

// The layout names the folders of polix.yaml, cleaned, or the defaults.
func TestWorkflowSessionContextLayout(t *testing.T) {
	g := Guidance{Layout: true}
	got := Workflow{Guidance: g}.SessionContext()
	for _, want := range []string{"\n  - ai/rules/: ", "\n  - ai/plans/: ", "\n  - ai/reports/: "} {
		if !strings.Contains(got, want) {
			t.Fatalf("defaults: %q lacks %q", got, want)
		}
	}
	w := Workflow{Guidance: g, Rules: Rules{Path: "./governance/rules/"}, Plans: Folder{Path: `docs\plans`},
		Reports: Folder{Path: "out/reports"}}
	got = w.SessionContext()
	for _, want := range []string{"\n  - governance/rules/: ", "\n  - docs/plans/: ", "\n  - out/reports/: "} {
		if !strings.Contains(got, want) {
			t.Fatalf("configured: %q lacks %q", got, want)
		}
	}
}

// An unknown key of guidance is a problem that names the keys allowed, also inside a workflow.
func TestGuidanceValid(t *testing.T) {
	g := Guidance{Commit: true, Keys: []string{"commit", "delegation"}}
	want := `unknown key "delegation" in guidance (commit, edit, layout)`
	if err := g.Valid(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("guidance: %v", err)
	}
	if err := (Workflow{Version: 1, Guidance: g}).Valid(); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("in a workflow: %v", err)
	}
	if err := (Guidance{Keys: []string{"commit", "edit", "layout"}}).Valid(); err != nil {
		t.Fatal(err)
	}
}
