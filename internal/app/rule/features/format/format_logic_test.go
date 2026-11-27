package format

import (
	"slices"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/rule"
)

const (
	formatted   = "rules:\n  - id: A-1\n    description: a\n"
	unformatted = "rules:\n    - description: a\n      id: A-1\n"
)

// Without files there is nothing to format.
func TestDecideEmpty(t *testing.T) {
	out := Decide(rule.Folder{Dir: "ai/rules"}, nil, false)
	if !out.Empty || out.Dir != "ai/rules" || out.Problems != nil || out.Changed != nil || out.Write != nil ||
		out.Fix != "" {
		t.Fatalf("got %+v", out)
	}
}

// Formatted files change nothing and leave nothing to write.
func TestDecideFormatted(t *testing.T) {
	files := map[string]rule.Source{"a.yaml": rule.Source(formatted), "b.yaml": rule.Source(formatted)}
	out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, nil, false)
	if out.Empty || out.Problems != nil || out.Changed != nil || out.Write != nil || out.Fix != "" {
		t.Fatalf("got %+v", out)
	}
}

// An unformatted file is a change, with its data unless check, and the fix polix rules fmt; retired.yaml is
// formatted like the rest.
func TestDecideUnformatted(t *testing.T) {
	files := map[string]rule.Source{"retired.yaml": rule.Source(unformatted), "a.yaml": rule.Source(formatted)}
	out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, nil, false)
	if !slices.Equal(out.Changed, []string{"ai/rules/retired.yaml"}) || out.Fix != "polix rules fmt" ||
		len(out.Write) != 1 || string(out.Write["retired.yaml"]) != formatted || out.Problems != nil {
		t.Fatalf("got %+v", out)
	}
	out = Decide(rule.Folder{Dir: "ai/rules", Files: files}, nil, true)
	if !slices.Equal(out.Changed, []string{"ai/rules/retired.yaml"}) || out.Fix != "polix rules fmt" ||
		out.Write != nil {
		t.Fatalf("check: %+v", out)
	}
}

// A file that cannot be formatted is a problem, and then no file is written at all, not even the ones that
// change; the problems and the changed files come sorted by name.
func TestDecideProblem(t *testing.T) {
	files := map[string]rule.Source{
		"d.yaml": rule.Source("rules:\n  - &x id: A-1\n"),
		"c.yaml": rule.Source(unformatted),
		"b.yaml": rule.Source("rules: [\n"),
		"a.yaml": rule.Source(unformatted),
	}
	out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, nil, false)
	if len(out.Problems) != 2 || !strings.HasPrefix(out.Problems[0], "ai/rules/b.yaml: ") ||
		!strings.HasPrefix(out.Problems[1], "ai/rules/d.yaml: ") || !strings.Contains(out.Problems[1], "anchors") {
		t.Fatalf("problems: %q", out.Problems)
	}
	if !slices.Equal(out.Changed, []string{"ai/rules/a.yaml", "ai/rules/c.yaml"}) || out.Write != nil {
		t.Fatalf("got %+v", out)
	}
}

// A repeated id is a problem, after those of the files, and then nothing is written, with or without check.
func TestDecideRepeatedIDs(t *testing.T) {
	repeat := "ai/rules/b.yaml A-1: id repeated, also in ai/rules/a.yaml"
	files := map[string]rule.Source{"a.yaml": rule.Source(formatted), "b.yaml": rule.Source(formatted)}
	for _, check := range []bool{false, true} {
		out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, []string{repeat}, check)
		if !slices.Equal(out.Problems, []string{repeat}) || out.Write != nil || out.Changed != nil {
			t.Fatalf("check %v: %+v", check, out)
		}
	}
	files["c.yaml"] = rule.Source(unformatted)
	out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, []string{repeat}, false)
	if !slices.Equal(out.Problems, []string{repeat}) || !slices.Equal(out.Changed, []string{"ai/rules/c.yaml"}) ||
		out.Write != nil {
		t.Fatalf("with an unformatted file: %+v", out)
	}
	files["0.yaml"] = rule.Source("rules: [\n")
	out = Decide(rule.Folder{Dir: "ai/rules", Files: files}, []string{repeat}, false)
	if len(out.Problems) != 2 || !strings.HasPrefix(out.Problems[0], "ai/rules/0.yaml: ") || out.Problems[1] != repeat {
		t.Fatalf("the repeats after the problems of the files: %q", out.Problems)
	}
}

// When the loader of the rules stops at a file, its error stands for the repeats: a file that formats but does not
// decode leaves nothing to write, and the error of a file that is not yaml, worded as its problem, is reported once.
func TestDecideLoaderError(t *testing.T) {
	stop := "ai/rules/b.yaml: yaml: unmarshal errors:\n  line 4: cannot unmarshal !!str `*.go` into []string"
	files := map[string]rule.Source{"a.yaml": rule.Source(unformatted),
		"b.yaml": rule.Source("rules:\n  - id: B-1\n    description: b\n    scope: \"*.go\"\n")}
	for _, check := range []bool{false, true} {
		out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, []string{stop}, check)
		if !slices.Equal(out.Problems, []string{stop}) || !slices.Equal(out.Changed, []string{"ai/rules/a.yaml"}) ||
			out.Write != nil {
			t.Fatalf("check %v: %+v", check, out)
		}
	}
	files["b.yaml"] = rule.Source("rules: [\n")
	first := Decide(rule.Folder{Dir: "ai/rules", Files: files}, nil, false).Problems
	if len(first) != 1 {
		t.Fatalf("a file that is not yaml: %q", first)
	}
	if out := Decide(rule.Folder{Dir: "ai/rules", Files: files}, first, false); !slices.Equal(out.Problems, first) || out.Write != nil {
		t.Fatalf("the same problem twice: %q", out.Problems)
	}
}
