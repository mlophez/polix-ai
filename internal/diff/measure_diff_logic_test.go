package diff

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/workflow"
)

// The logic narrows the diff to the globs and weighs each file with the first matching weight.
func TestMeasureDiffLogic(t *testing.T) {
	d := Diff{Changes: []Change{
		{Path: "internal/hook/a.go", Added: 5, Deleted: 2},
		{Path: "internal/hook/a_test.go", Added: 15},
		{Path: "docs/polix.md", Added: 40},
		{Path: "logo.png", Binary: true},
	}}
	weights := []workflow.Weight{{Glob: "*_test.go", Factor: 0.5}, {Glob: "internal/hook/**", Factor: 3}}
	out := MeasureDiff(d, weights, []string{"*.go"})
	// 3 * log2(8) + 0.5 * log2(16) = 9 + 2 = 11
	if out.Lines != 22 || out.Files != 2 || out.Risk != 11 {
		t.Fatalf("totals = %d %d %d", out.Lines, out.Files, out.Risk)
	}
	if len(out.Contributions) != 2 || out.Contributions[0].Path != "internal/hook/a.go" ||
		out.Changes[0].Added != 5 || out.Changes[0].Deleted != 2 || out.Changes[1].Path != "internal/hook/a_test.go" {
		t.Fatalf("rows = %+v %+v", out.Contributions, out.Changes)
	}
	all := MeasureDiff(d, nil, nil)
	if all.Lines != 62 || all.Files != 4 || len(all.Changes) != 4 || all.Changes[3].Path != "logo.png" {
		t.Fatalf("all = %+v", all)
	}
	if empty := MeasureDiff(Diff{}, weights, nil); empty.Lines != 0 || empty.Files != 0 || empty.Risk != 0 {
		t.Fatalf("empty = %+v", empty)
	}
}
