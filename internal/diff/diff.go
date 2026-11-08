// Package diff measures a change of a repository: the lines and the files it touches, and a risk score that
// weighs the lines of each file by where it lives. The measures are pure: they take the size of each change and
// the weights of the workflow, and never read git themselves.
package diff

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/mlophez/polix-ai/internal/workflow"
	"github.com/mlophez/polix-ai/pkg/glob"
)

// Diff is a change of the repository, one Change per file.
type Diff struct {
	Changes []Change
}

// Change is the size of the change of one file: its added and deleted lines, or Binary with both at zero.
type Change struct {
	Path    string // relative to the root of the repository, with forward slashes
	Added   int
	Deleted int
	Binary  bool
}

// Contribution is the share of one file in the risk score: Factor times log2(1 + Lines).
type Contribution struct {
	Path   string
	Lines  int
	Factor float64
	Score  float64
}

// DefaultFactor is the weight of a file that no glob of the weights matches.
const DefaultFactor = 1.0

// Valid tells why a diff is not valid: an invalid change or a path repeated.
func (d Diff) Valid() error {
	seen := map[string]bool{}
	for _, c := range d.Changes {
		if err := c.Valid(); err != nil {
			return err
		}
		if seen[c.Path] {
			return fmt.Errorf("diff: path %q repeated", c.Path)
		}
		seen[c.Path] = true
	}
	return nil
}

// Valid tells why a change is not valid: no path, a negative count, or a binary change with lines.
func (c Change) Valid() error {
	switch {
	case c.Path == "":
		return errors.New("change: empty path")
	case c.Added < 0 || c.Deleted < 0:
		return fmt.Errorf("change %q: negative line count", c.Path)
	case c.Binary && (c.Added != 0 || c.Deleted != 0):
		return fmt.Errorf("change %q: a binary change counts no lines", c.Path)
	}
	return nil
}

// Valid tells why a contribution is not valid: no path, negative lines, or a negative or non-finite factor or score.
func (c Contribution) Valid() error {
	switch {
	case c.Path == "":
		return errors.New("contribution: empty path")
	case c.Lines < 0:
		return fmt.Errorf("contribution %q: negative line count", c.Path)
	case math.IsNaN(c.Factor) || math.IsInf(c.Factor, 0) || c.Factor < 0:
		return fmt.Errorf("contribution %q: invalid factor", c.Path)
	case math.IsNaN(c.Score) || math.IsInf(c.Score, 0) || c.Score < 0:
		return fmt.Errorf("contribution %q: invalid score", c.Path)
	}
	return nil
}

// Narrow keeps the changes whose path matches any of the globs; no globs keep the whole diff.
func (d Diff) Narrow(patterns []string) Diff {
	out := d
	if len(patterns) == 0 {
		out.Changes = slices.Clone(d.Changes)
		return out
	}
	var kept []Change
	for _, c := range d.Changes {
		if glob.AnyPath(patterns, c.Path) {
			kept = append(kept, c)
		}
	}
	out.Changes = kept
	return out
}

// Lines sums the added and deleted lines of the changes.
func (d Diff) Lines() int {
	n := 0
	for _, c := range d.Changes {
		n += c.Lines()
	}
	return n
}

// Count returns how many files changed, deleted and binary included.
func (d Diff) Count() int {
	return len(d.Changes)
}

// Lines sums the added and deleted lines of the change.
func (c Change) Lines() int {
	return c.Added + c.Deleted
}

// Contributions returns the share of each change in the risk score, the highest first, then by path. The factor
// of a change is that of the first weight whose glob matches it, else DefaultFactor. An invalid weight, which
// polix config check reports, is skipped, and so is an invalid change, which Diff.Valid rejects.
func (d Diff) Contributions(weights []workflow.Weight) []Contribution {
	out := make([]Contribution, 0, len(d.Changes))
	for _, c := range d.Changes {
		factor := DefaultFactor
		for _, w := range weights {
			if w.Valid() == nil && glob.Path(w.Glob, c.Path) {
				factor = w.Factor
				break
			}
		}
		lines := c.Lines()
		ct := Contribution{
			Path:   c.Path,
			Lines:  lines,
			Factor: factor,
			Score:  factor * math.Log2(1+float64(lines)),
		}
		if err := ct.Valid(); err != nil {
			continue
		}
		out = append(out, ct)
	}
	slices.SortFunc(out, func(a, b Contribution) int {
		if a.Score != b.Score {
			if a.Score > b.Score {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}

// Risk is the sum of the scores of the contributions, rounded half away from zero.
func (d Diff) Risk(weights []workflow.Weight) int {
	sum := 0.0
	for _, c := range d.Contributions(weights) {
		sum += c.Score
	}
	return int(math.Round(sum))
}
