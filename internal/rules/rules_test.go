package rules

import "testing"

func TestMatching(t *testing.T) {
	all := []Rule{
		{ID: "ARCH-1", Scope: []string{"src/domain/**"}},
		{ID: "ARCH-2"}, // no scope: the whole project
		{ID: "STYLE-1", Scope: []string{"**/*.tf"}},
	}
	got := Matching(all, []string{"src/domain/order.go"})
	if len(got) != 2 || got[0].ID != "ARCH-1" || got[1].ID != "ARCH-2" {
		t.Fatalf("Matching = %v", got)
	}
	if got := Matching(all, nil); len(got) != 0 {
		t.Fatalf("no paths, no rules: %v", got)
	}
}
