package yamlfmt

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The layout: two spaces, sequences below their key, flow collections with spaces inside braces, one blank line at
// most, the quotes and the block scalars of the source, and every comment where it was.
func TestFormatLayout(t *testing.T) {
	src := `# header


# about a
a:
    b: 1   # one
    c: { x:  1,  y: "polix:reviewer" }
    d: [ a,  b ]



    e:
    - 'it''s'
    - "x\ty"
    - |
      keep
        this
    # deeper, below the last item of e
f: >-
    folded
    text
# end
`
	want := `# header

# about a
a:
  b: 1 # one
  c: { x: 1, y: "polix:reviewer" }
  d: [a, b]

  e:
    - 'it''s'
    - "x\ty"
    - |
      keep
        this
    # deeper, below the last item of e
f: >-
  folded
  text
# end
`
	got, err := Format([]byte(src), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The keys follow the order, with the unknown ones after in source order; the comments above a key travel with it,
// the header of the document stays at the top, and the items of a sequence never move.
func TestFormatOrder(t *testing.T) {
	src := `# header

# about z
z: 1
b: 2

# about a
a:
  - { q: 1, p: 2 }
  - q: 3
    p: 4
`
	order := func(path []string) []string {
		switch strings.Join(path, "/") {
		case "":
			return []string{"a", "z"}
		case "a/-":
			return []string{"p", "q"}
		}
		return nil
	}
	want := `# header

# about a
a:
  - { p: 2, q: 1 }
  - p: 4
    q: 3
# about z
z: 1
b: 2
`
	got, err := Format([]byte(src), order, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The items of a spaced sequence are separated by exactly one blank line, above the comments of an item and never
// after the key of the sequence; the blank lines inside an item stay, and a nested sequence is not spaced. The output
// formats to itself, and on \r\n input the blank line is \r\n too.
func TestFormatSpaced(t *testing.T) {
	src := `rules:
  - id: A-1
    scope:
      - a
      - b
  - id: A-2

    description: kept blank line inside the item


  - id: A-3
  # about A-4
  - id: A-4
`
	want := `rules:
  - id: A-1
    scope:
      - a
      - b

  - id: A-2

    description: kept blank line inside the item

  - id: A-3

  # about A-4
  - id: A-4
`
	spaced := func(path []string) bool { return strings.Join(path, "/") == "rules" }
	got, err := Format([]byte(src), nil, spaced)
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := Format(got, nil, spaced); err != nil || string(twice) != want {
		t.Fatalf("not idempotent:\n%s\n%v", twice, err)
	}
	crlf := strings.ReplaceAll(src, "\n", "\r\n")
	if got, err := Format([]byte(crlf), nil, spaced); err != nil || string(got) != strings.ReplaceAll(want, "\n", "\r\n") {
		t.Fatalf("crlf: %q %v", got, err)
	}
	if got, err := Format([]byte(src), nil, nil); err != nil || strings.Contains(string(got), "b\n\n  - id: A-2") {
		t.Fatalf("without spaced, no blank line is added:\n%s\n%v", got, err)
	}
	// A blank line below a block scalar with the indicator + would be its content: an item that ends in one counts
	// as separated already, and one whose keep scalar is not its last value does not.
	keep := "rules:\n  - id: A-1\n    description: |+\n      literal\n  - id: A-2\n    description: >+\n      folded\n" +
		"  - id: A-3\n    description: |+\n      kept\n    reason: r\n  - id: A-4\n"
	want = strings.Replace(keep, "  - id: A-4", "\n  - id: A-4", 1)
	got, err = Format([]byte(keep), nil, spaced)
	if err != nil || string(got) != want {
		t.Fatalf("keep:\ngot:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := Format(got, nil, spaced); err != nil || string(twice) != want {
		t.Fatalf("keep, not idempotent:\n%s\n%v", twice, err)
	}
}

// Two items of a spaced sequence written on one line each stay together; a blank line goes between them only when one
// of the two takes more lines or has a comment, and a blank line of the source between them stays.
func TestFormatSpacedOneLine(t *testing.T) {
	src := `items:
  - a: 1
  - b
  - { c: 3 }
  - d: 4
    e: 5
  - f: 6
  - g: 7 # line comment
  # about h
  - h: 8
  - i: 9

  - j: 10
  - k: |
      body
  - l: 11
  - m: 12
    # deeper, below m
  - n: 13
`
	want := `items:
  - a: 1
  - b
  - { c: 3 }

  - d: 4
    e: 5

  - f: 6
  - g: 7 # line comment

  # about h
  - h: 8

  - i: 9

  - j: 10

  - k: |
      body

  - l: 11

  - m: 12
    # deeper, below m

  - n: 13
`
	spaced := func(path []string) bool { return strings.Join(path, "/") == "items" }
	got, err := Format([]byte(src), nil, spaced)
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := Format(got, nil, spaced); err != nil || string(twice) != want {
		t.Fatalf("not idempotent:\n%s\n%v", twice, err)
	}
}

// A formatted document comes back as it is, line breaks \r\n included.
func TestFormatIdempotent(t *testing.T) {
	src := "a:\r\n  - b # c\r\n"
	got, err := Format([]byte(src), nil, nil)
	if err != nil || string(got) != src {
		t.Fatalf("%q %v", got, err)
	}
}

// An empty document or one with only comments comes back as it is; broken YAML is an error.
func TestFormatEdges(t *testing.T) {
	for _, src := range []string{"", "# only\n", "scalar\n"} {
		if got, err := Format([]byte(src), nil, nil); err != nil || string(got) != src {
			t.Fatalf("%q: %q %v", src, got, err)
		}
	}
	if _, err := Format([]byte("a: [\n"), nil, nil); err == nil {
		t.Fatal("broken yaml")
	}
}

// A leading document start marker stays, with the comments above it, and the rest is formatted.
func TestFormatDocumentStart(t *testing.T) {
	for src, want := range map[string]string{
		"---\nb:    1\n":              "---\nb: 1\n",
		"# top\n\n---\r\nb:    1\r\n": "# top\r\n\r\n---\r\nb: 1\r\n",
		"---\n":                       "---\n",
	} {
		if got, err := Format([]byte(src), nil, nil); err != nil || string(got) != want {
			t.Fatalf("%q: %q %v", src, got, err)
		}
	}
}

// Merge keys that use an anchor, tags, complex keys, directives and any other document marker are refused.
func TestFormatUnsupported(t *testing.T) {
	merge := "a: &x { k: 1 }\nb:\n  <<: *x\n"
	if _, err := Format([]byte(merge), nil, nil); !errors.Is(err, ErrUnsupported) ||
		err.Error() != "line 3: anchors and aliases are not supported by the formatter" {
		t.Fatalf("merge: %v", err)
	}
	for _, src := range []string{"a: !!str 1\n", "? [a]\n: 1\n", "---\na: 1\n---\nb: 2\n",
		"%YAML 1.2\n---\na: 1\n", "a: 1\n...\n"} {
		if _, err := Format([]byte(src), nil, nil); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: %v", src, err)
		}
	}
}

// The anchors and aliases of the values stay where they are written, on every kind of value: plain, quoted and block
// scalars, flow collections, and block mappings and sequences, as the value of a key and as an item of a sequence, an
// alias inside a flow collection included. A second pass changes nothing.
func TestFormatAnchors(t *testing.T) {
	for name, c := range map[string][2]string{
		"plain":           {"a:   &x 1\nb: *x\n", "a: &x 1\nb: *x\n"},
		"null":            {"a: &x\nb: *x\n", "a: &x\nb: *x\n"},
		"double quoted":   {"a: &x   \"it's  q\"\nb: *x\n", "a: &x \"it's  q\"\nb: *x\n"},
		"single quoted":   {"a: &x 'it''s'\nb: *x\n", "a: &x 'it''s'\nb: *x\n"},
		"block scalar":    {"a: &x |\n    keep\n      this\nb: *x\n", "a: &x |\n  keep\n    this\nb: *x\n"},
		"block indicator": {"a: &x >-\n    x\n    y\nb: 1\n", "a: &x >-\n  x\n  y\nb: 1\n"},
		"flow mapping":    {"a: &m {  k: 1 }\nb: *m\n", "a: &m { k: 1 }\nb: *m\n"},
		"flow sequence":   {"a: &s [ 1,  2 ]\nb: *s\n", "a: &s [1, 2]\nb: *s\n"},
		"empty flow":      {"a: &m {}\nb: *m\n", "a: &m {}\nb: *m\n"},
		"block mapping":   {"a: &m\n    k: 1\n    j: 2\nb: *m\n", "a: &m\n  k: 1\n  j: 2\nb: *m\n"},
		"block sequence":  {"a: &s\n- 1\n- 2\nb: *s\n", "a: &s\n  - 1\n  - 2\nb: *s\n"},
		"mapping item": {"l:\n  - &m\n      k: 1\n      j: 2\n  - *m\n",
			"l:\n  - &m\n    k: 1\n    j: 2\n  - *m\n"},
		"sequence item":     {"l:\n  - &s\n    - 1\n    - 2\n  - *s\n", "l:\n  - &s\n    - 1\n    - 2\n  - *s\n"},
		"scalar item":       {"l:\n  - &x   1\n  - *x\n", "l:\n  - &x 1\n  - *x\n"},
		"block after break": {"a: &x\n  |\n  text\nb: *x\n", "a: &x |\n  text\nb: *x\n"},
		"block in sequence": {"a: &s\n- |\n x\n- y\nb: *s\n", "a: &s\n  - |\n    x\n  - y\nb: *s\n"},
		"block item":        {"l:\n  - &x |\n     a\n  - *x\n", "l:\n  - &x |\n    a\n  - *x\n"},
		"nested":            {"l:\n  - &m\n    k: &x 1\n    j: &s\n      - *x\n", "l:\n  - &m\n    k: &x 1\n    j: &s\n      - *x\n"},
		"in flow":           {"a: &q x\nb: [*q, y]\nc: { k: *q }\n", "a: &q x\nb: [*q, y]\nc: { k: *q }\n"},
		"anchor in flow": {"a: [ &q x,  *q ]\nb: { k: &r 1, j: *r }\n",
			"a: [&q x, *q]\nb: { k: &r 1, j: *r }\n"},
		"line comments": {"a: &x 1 # one\nb: *x # two\n", "a: &x 1 # one\nb: *x   # two\n"},
	} {
		got, err := Format([]byte(c[0]), nil, nil)
		if err != nil || string(got) != c[1] {
			t.Fatalf("%s:\ngot:\n%s\nwant:\n%s\n%v", name, got, c[1], err)
		}
		if twice, err := Format(got, nil, nil); err != nil || string(twice) != c[1] {
			t.Fatalf("%s: not idempotent:\n%s\n%v", name, twice, err)
		}
	}
}

// An anchored block collection is written alone on the line of its dash, never with its first key after the anchor,
// which yaml.v3 would read as an anchor on the key.
func TestFormatAnchorOnItsOwnLine(t *testing.T) {
	got, err := Format([]byte("l:\n   -   &m\n         k: 1\n         j: 2\n"), nil, nil)
	if err != nil || string(got) != "l:\n  - &m\n    k: 1\n    j: 2\n" {
		t.Fatalf("%q %v", got, err)
	}
}

// A block scalar whose header is on the line below its anchor keeps its block style and its content, even a line
// that starts with #, and comes out with the header on the line of its anchor.
func TestFormatAnchorBlockHeaderBelow(t *testing.T) {
	for src, want := range map[string]string{
		"a: &x\n  |\n  # not a comment\nb: *x\n":          "a: &x |\n  # not a comment\nb: *x\n",
		"a: &x\n\n  >-\n    one\n    two\nb: *x\n":        "a: &x >-\n  one\n  two\nb: *x\n",
		"l:\n  - &x\n    |2\n       text\n  - *x\n":       "l:\n  - &x |2\n       text\n  - *x\n",
		"a: &x\n  |+\n  text\n\nb: *x\n":                  "a: &x |+\n  text\n\nb: *x\n",
		"a: &x\n  | # about a\n  # not a comment\nb: 1\n": "a: &x | # about a\n  # not a comment\nb: 1\n",
	} {
		got, err := Format([]byte(src), nil, nil)
		if err != nil || string(got) != want {
			t.Errorf("%q: got %q, want %q, %v", src, got, want, err)
			continue
		}
		if twice, err := Format(got, nil, nil); err != nil || string(twice) != want {
			t.Errorf("%q: not idempotent: %q %v", src, twice, err)
		}
	}
}

// The comments of an anchored sequence item stay where they were: the one above its second key above that key, and
// the one below its last line, indented deeper than the next item, below it.
func TestFormatAnchorComments(t *testing.T) {
	src := "l:\n  - &e\n      a: 1\n      # about b\n      b: 2\n      # below e\n  - c\n"
	want := "l:\n  - &e\n    a: 1\n    # about b\n    b: 2\n      # below e\n  - c\n"
	got, err := Format([]byte(src), nil, nil)
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := Format(got, nil, nil); err != nil || string(twice) != want {
		t.Fatalf("not idempotent:\n%s\n%v", twice, err)
	}
	// An anchored sequence: the comments between its items and the one above its first item.
	src = "a: &s\n  # about 1\n  - 1\n  # about 2\n  - 2\nb: 1\n"
	if got, err := Format([]byte(src), nil, nil); err != nil || string(got) != src {
		t.Fatalf("sequence:\n%s\n%v", got, err)
	}
	// A comment above an anchored item, and a block scalar below an anchored sequence with explicit indentation.
	src = "l:\n  # about e\n  - &e\n    a: 1\n  - &s\n    - |2\n          text\n"
	want = "l:\n  # about e\n  - &e\n    a: 1\n  - &s\n    - |2\n          text\n"
	if got, err := Format([]byte(src), nil, nil); err != nil || string(got) != want {
		t.Fatalf("above:\n%s\n%v", got, err)
	}
}

// An anchor or alias in front of a key, an anchor on the whole document and an anchor name defined twice are refused
// with their line.
func TestFormatAnchorRefusals(t *testing.T) {
	for src, want := range map[string]string{
		"&k a: 1\n":                "line 1: anchors and aliases on keys are not supported by the formatter",
		"l:\n  - &f when: 1\n":     "line 2: anchors and aliases on keys are not supported by the formatter",
		"x: &k a\n*k: 1\n":         "line 2: anchors and aliases on keys are not supported by the formatter",
		"&r\na: 1\n":               "line 1: an anchor on the whole document is not supported by the formatter",
		"&r\n- a\n":                "line 1: an anchor on the whole document is not supported by the formatter",
		"a: &x 1\nb:\n  c: &x 2\n": "line 3: the anchor &x is defined twice: rename one",
	} {
		_, err := Format([]byte(src), nil, nil)
		if !errors.Is(err, ErrUnsupported) || err.Error() != want {
			t.Fatalf("%q: got %v, want %s", src, err, want)
		}
	}
}

// A comment above a key that the order moves first in a sequence item goes above the dash, and a second pass keeps
// it there.
func TestFormatStableInSequenceItem(t *testing.T) {
	order := func(path []string) []string {
		if strings.Join(path, "/") == "on/stop/-" {
			return []string{"when", "action"}
		}
		return nil
	}
	src := "on:\n  stop:\n    - action: { block: x }\n      # about when\n      when: { changed: x }\n"
	want := "on:\n  stop:\n    # about when\n    - when: { changed: x }\n      action: { block: x }\n"
	for _, in := range []string{src, want, "-\n  # c\n  a: 1\n"} {
		once, err := Format([]byte(in), order, nil)
		if err != nil {
			t.Fatal(err)
		}
		if in == src && string(once) != want {
			t.Fatalf("got:\n%s", once)
		}
		if twice, err := Format(once, order, nil); err != nil || string(twice) != string(once) {
			t.Fatalf("not stable: %q -> %q %v", once, twice, err)
		}
	}
}

// Block scalars keep their indicators and content: explicit indentation, keep, strip, and \r\n.
func TestFormatBlockScalars(t *testing.T) {
	for src, want := range map[string]string{
		"a:\n    b: |2\n         x\n        y\n":      "a:\n  b: |2\n       x\n      y\n",
		"a: |+\n  x\n\nb: 1\n":                        "a: |+\n  x\n\nb: 1\n",
		"a: >-\n    x\n    y\nb: 1\n":                 "a: >-\n  x\n  y\nb: 1\n",
		"a:\r\n    b: |\r\n      # not a comment\r\n": "a:\r\n  b: |\r\n    # not a comment\r\n",
	} {
		got, err := Format([]byte(src), nil, nil)
		if err != nil || string(got) != want {
			t.Fatalf("%q: %q %v", src, got, err)
		}
	}
}

// A comment inside a flow collection is refused; one after it, or a # inside a quoted or plain scalar, is not.
func TestFormatFlowComments(t *testing.T) {
	for _, src := range []string{"a: [ # c\n  x ]\n", "a: { # c\n  x: 1 }\n", "a: [x, # c\n  y]\n"} {
		if _, err := Format([]byte(src), nil, nil); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("%q: %v", src, err)
		}
	}
	for _, src := range []string{"a: [x, \"#y\", it's#z] # c\n", "a: { b: 'x # y' }\n"} {
		if got, err := Format([]byte(src), nil, nil); err != nil || string(got) != src {
			t.Fatalf("%q: %q %v", src, got, err)
		}
	}
}

// Line comments line up by characters, not bytes.
func TestFormatAlignsRunes(t *testing.T) {
	got, err := Format([]byte("ñ: ññ # c1\nbb: y # c2\n"), nil, nil)
	if err != nil || string(got) != "ñ: ññ # c1\nbb: y # c2\n" {
		t.Fatalf("%q %v", got, err)
	}
}

// The safety net refuses an output that changes the data or loses a comment.
func TestSame(t *testing.T) {
	if err := same("a: 1\n", "a: 2\n"); err == nil || !strings.Contains(err.Error(), "change the data") {
		t.Fatal(err)
	}
	if err := same("a: 1 # c\n", "a: 1\n"); err == nil || !strings.Contains(err.Error(), "lose a comment") {
		t.Fatal(err)
	}
	if err := same("a: 1 # c\n", "a: 1 # c\n"); err != nil {
		t.Fatal(err)
	}
}

// The workflows of the repository format to themselves twice: the first pass keeps their data and comments, the
// second changes nothing.
func TestFormatRepository(t *testing.T) {
	for _, name := range []string{"polix.yaml", "examples/polix.yaml"} {
		src, err := os.ReadFile(filepath.Join("..", "..", "..", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		once, err := Format(src, nil, nil)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		twice, err := Format(once, nil, nil)
		if err != nil || string(twice) != string(once) {
			t.Fatalf("%s: not idempotent: %v", name, err)
		}
	}
}
