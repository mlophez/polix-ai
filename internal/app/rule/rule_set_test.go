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
	if got := Rules(all).Repeated(); !slices.Equal(got, want) {
		t.Fatalf("Repeated = %q, want %q", got, want)
	}
	if got := Rules(all[2:]).Repeated(); len(got) != 0 {
		t.Fatalf("no repeated id: %q", got)
	}
}
