package diff

import (
	"math"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/workflow"
)

func sample() Diff {
	return Diff{Changes: []Change{
		{Path: "internal/hook/a.go", Added: 10, Deleted: 5},
		{Path: "internal/hook/a_test.go", Added: 7},
		{Path: "docs/polix.md", Added: 3, Deleted: 0},
		{Path: "gone.go", Deleted: 4},
		{Path: "logo.png", Binary: true},
	}}
}

func paths(d Diff) []string {
	var out []string
	for _, c := range d.Changes {
		out = append(out, c.Path)
	}
	return out
}

// Narrow keeps the changes matching a glob; no globs keep everything.
func TestDiffNarrow(t *testing.T) {
	d := sample()
	if got := paths(d.Narrow(nil)); !slices.Equal(got, paths(d)) {
		t.Fatalf("Narrow(nil) = %q", got)
	}
	if got := paths(d.Narrow([]string{"*.go"})); !slices.Equal(got, []string{"internal/hook/a.go", "internal/hook/a_test.go", "gone.go"}) {
		t.Fatalf("Narrow(*.go) = %q", got)
	}
	if got := paths(d.Narrow([]string{"/gone.go", "docs/**"})); !slices.Equal(got, []string{"docs/polix.md", "gone.go"}) {
		t.Fatalf("Narrow(anchored) = %q", got)
	}
}

// Lines sums added and deleted lines; Count counts deleted and binary files too.
func TestDiffLinesCount(t *testing.T) {
	d := sample()
	if d.Lines() != 29 || d.Count() != 5 {
		t.Fatalf("Lines, Count = %d, %d", d.Lines(), d.Count())
	}
	if (Diff{}).Lines() != 0 || (Diff{}).Count() != 0 {
		t.Fatal("an empty diff measures zero")
	}
}

// The first matching weight wins, a zero weight cancels a file, and an unmatched file weighs 1.
func TestDiffRisk(t *testing.T) {
	d := Diff{Changes: []Change{
		{Path: "internal/hook/a_test.go", Added: 3}, // 0.5 * 2 = 1
		{Path: "internal/hook/a.go", Added: 7},      // 3 * 3 = 9
		{Path: "docs/x.md", Added: 15},              // 0
		{Path: "other.go", Added: 1},                // 1 * 1 = 1
	}}
	weights := []workflow.Weight{
		{Glob: "*_test.go", Factor: 0.5},
		{Glob: "internal/hook/**", Factor: 3},
		{Glob: "docs/**", Factor: 0},
		{Glob: "", Factor: 9}, // invalid: skipped
	}
	if got := d.Risk(weights); got != 11 {
		t.Fatalf("Risk = %d, want 11", got)
	}
	if got := (Diff{}).Risk(weights); got != 0 {
		t.Fatalf("Risk of an empty diff = %d", got)
	}
	half := Diff{Changes: []Change{{Path: "a", Added: 1}}} // 2.5 * log2(2) = 2.5
	if got := half.Risk([]workflow.Weight{{Glob: "a", Factor: 2.5}}); got != 3 {
		t.Fatalf("Risk rounding at .5 = %d, want 3", got)
	}
}

// Contributions are sorted by score, highest first, then by path.
func TestDiffContributions(t *testing.T) {
	d := Diff{Changes: []Change{
		{Path: "b.go", Added: 1},
		{Path: "a.go", Added: 1},
		{Path: "big.go", Added: 7},
		{Path: "bin", Binary: true},
	}}
	got := d.Contributions(nil)
	var order []string
	for _, c := range got {
		order = append(order, c.Path)
	}
	if !slices.Equal(order, []string{"big.go", "a.go", "b.go", "bin"}) {
		t.Fatalf("order = %q", order)
	}
	if got[0].Lines != 7 || got[0].Factor != 1 || got[0].Score != 3 {
		t.Fatalf("contribution = %+v", got[0])
	}
}

func TestDiffValid(t *testing.T) {
	if err := sample().Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Diff{Changes: []Change{{Path: "a"}, {Path: "a"}}}).Valid(); err == nil {
		t.Fatal("a repeated path is not valid")
	}
	if err := (Diff{Changes: []Change{{}}}).Valid(); err == nil {
		t.Fatal("an invalid change makes the diff invalid")
	}
}

func TestChangeValid(t *testing.T) {
	for _, c := range []Change{{}, {Path: "a", Added: -1}, {Path: "a", Deleted: -1}, {Path: "a", Binary: true, Added: 1}} {
		if c.Valid() == nil {
			t.Errorf("%+v is valid", c)
		}
	}
	if err := (Change{Path: "a", Binary: true}).Valid(); err != nil {
		t.Fatal(err)
	}
}

func TestContributionValid(t *testing.T) {
	for _, c := range []Contribution{{}, {Path: "a", Lines: -1}, {Path: "a", Factor: -1}, {Path: "a", Score: math.NaN()}} {
		if c.Valid() == nil {
			t.Errorf("%+v is valid", c)
		}
	}
	if err := (Contribution{Path: "a", Lines: 1, Factor: 1, Score: 1}).Valid(); err != nil {
		t.Fatal(err)
	}
}
