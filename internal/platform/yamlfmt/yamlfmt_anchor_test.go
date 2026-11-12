package yamlfmt

import (
	"errors"
	"testing"
)

// An order that would put an alias before its anchor fails with the line of the alias, and writes nothing.
func TestFormatAliasBeforeAnchor(t *testing.T) {
	src := "a: &x 1\nb: *x\n"
	want := "line 2: the alias *x would come before its anchor &x: move the anchor up"
	bFirst := func(path []string) []string {
		if len(path) == 0 {
			return []string{"b", "a"}
		}
		return nil
	}
	got, err := Format([]byte(src), bFirst, nil)
	if !errors.Is(err, ErrUnsupported) || err.Error() != want || got != nil {
		t.Fatalf("got %q, %v, want %s", got, err, want)
	}
	aFirst := func(path []string) []string {
		if len(path) == 0 {
			return []string{"a", "b"}
		}
		return nil
	}
	if got, err := Format([]byte(src), aFirst, nil); err != nil || string(got) != src {
		t.Fatalf("anchor first: %q %v", got, err)
	}
}

// An alias nested in a section that the order moves above the section of its anchor, as in a polix.yaml with githooks
// written before on, fails with the line of the alias; with the anchor in the section that stays first it formats.
func TestFormatAliasInLaterSection(t *testing.T) {
	order := func(path []string) []string {
		if len(path) == 0 {
			return []string{"version", "on", "githooks"}
		}
		return nil
	}
	src := `githooks:
  pre_commit:
    - action:
        run: &test
          command: just test
on:
  stop:
    - action:
        run: *test
`
	want := "line 9: the alias *test would come before its anchor &test: move the anchor up"
	if _, err := Format([]byte(src), order, nil); !errors.Is(err, ErrUnsupported) || err.Error() != want {
		t.Fatalf("got %v, want %s", err, want)
	}
	moved := `on:
  stop:
    - action:
        run: &test
          command: just test
githooks:
  pre_commit:
    - action:
        run: *test
`
	if got, err := Format([]byte(moved), order, nil); err != nil || string(got) != moved {
		t.Fatalf("anchor first:\n%s\n%v", got, err)
	}
	// An alias inside a flow collection counts too.
	bFirst := func(path []string) []string {
		if len(path) == 0 {
			return []string{"b", "a"}
		}
		return nil
	}
	want = "line 2: the alias *x would come before its anchor &x: move the anchor up"
	if _, err := Format([]byte("a: &x 1\nb: [*x]\n"), bFirst, nil); err == nil || err.Error() != want {
		t.Fatalf("flow: %v", err)
	}
}
