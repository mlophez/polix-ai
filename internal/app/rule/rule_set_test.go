package rule

import (
	"slices"
	"testing"
)

func TestMatching(t *testing.T) {
	all := []Rule{
		{ID: "ARCH-1", Scope: []string{"src/domain/**"}},
		{ID: "ARCH-2"}, // no scope: the whole project
		{ID: "STYLE-1", Scope: []string{"**/*.tf"}},
	}
	got := Rules(all).Matching([]string{"src/domain/order.go"})
	if len(got) != 2 || got[0].ID != "ARCH-1" || got[1].ID != "ARCH-2" {
		t.Fatalf("Matching = %v", got)
	}
	if got := Rules(all).Matching([]string{"README.md"}); len(got) != 1 || got[0].ID != "ARCH-2" {
		t.Fatalf("a file out of every scope: %v", got)
	}
	if got := Rules(all).Matching(nil); len(got) != 0 {
		t.Fatalf("no paths, no rules: %v", got)
	}
}

func TestRepeated(t *testing.T) {
	all := []Rule{
		{ID: "A-1", File: "ai/rules/a.yaml"},
		{ID: "A-1", File: "ai/rules/b.yaml"},
		{ID: "B-1", File: "ai/rules/b.yaml"},
		{File: "ai/rules/c.yaml"},
	}
	want := []string{"ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"}
	if got := Rules(all).Repeated(nil); !slices.Equal(got, want) {
		t.Fatalf("Repeated = %q, want %q", got, want)
	}
	if got := Rules(all[2:]).Repeated(nil); len(got) != 0 {
		t.Fatalf("no repeated id: %q", got)
	}
}

// The ids of the retired rules are taken before the active rules: an active rule with one of them is repeated,
// naming the retired file, and a repeat among the retired rules themselves is never reported.
func TestRepeatedRetired(t *testing.T) {
	retired := Rules{
		{ID: "GOARCH-31", File: "ai/rules/retired.yaml"},
		{ID: "GOARCH-31", File: "ai/rules/retired.yaml"},
	}
	active := Rules{{ID: "GOARCH-31", File: "ai/rules/a.yaml"}, {ID: "GOARCH-1", File: "ai/rules/a.yaml"}}
	want := []string{"ai/rules/a.yaml GOARCH-31: id repeated, also in ai/rules/retired.yaml"}
	if got := active.Repeated(retired); !slices.Equal(got, want) {
		t.Fatalf("Repeated = %q, want %q", got, want)
	}
	if got := Rules(nil).Repeated(retired); len(got) != 0 {
		t.Fatalf("a repeat among the retired rules: %q", got)
	}
}

func TestSelection(t *testing.T) {
	rs := Rules{{ID: "A-1", Scope: []string{"*.go"}}, {ID: "B-1", Scope: []string{"docs/**"}}}
	every := Selection{}
	if err := every.Valid(); err != nil || len(every.Narrow(rs)) != 2 {
		t.Fatalf("every rule: %v %v", err, every.Narrow(rs))
	}
	changed := Selection{OnlyChanged: true, Changed: []string{"main.go"}}
	if got := changed.Narrow(rs); changed.Valid() != nil || len(got) != 1 || got[0].ID != "A-1" {
		t.Fatalf("only the rules of the changed files: %v", got)
	}
	if none := (Selection{OnlyChanged: true}).Narrow(rs); len(none) != 0 {
		t.Fatalf("nothing changed selects no rule: %v", none)
	}
	if (Selection{Changed: []string{"main.go"}}).Valid() == nil {
		t.Fatal("changed files without OnlyChanged are valid")
	}
}
