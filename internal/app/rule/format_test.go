package rule

import (
	"testing"
)

// rules goes first, the keys of a rule follow the schema with the unknown ones after (reason among them), and the rules never move; a
// comment above a key moves with it, a flow scope on two lines ends on one and a folded description keeps its text.
func TestFormat(t *testing.T) {
	src := `x-owner: platform
rules:
  - check: tests/a_test.go
    scope: ["a/**",
      "b/**"]
    x-note: kept last
    reason: why
    # the id of the rule
    id: B-1
    deterministic: true
    description: >-
      folded
      text
  - id: A-1
    description: d
`
	want := `rules:
  # the id of the rule
  - id: B-1
    description: >-
      folded
      text
    scope: ["a/**", "b/**"]
    deterministic: true
    check: tests/a_test.go
    x-note: kept last
    reason: why

  - id: A-1
    description: d
x-owner: platform
`
	got, err := Source(src).Format()
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
}

// Three rules written without blank lines come out with one blank line between each two, above the comments of a
// rule, and none after rules:; the scope of a rule in block style gets no blank line.
func TestFormatSpacesTheRules(t *testing.T) {
	src := `rules:
  - id: A-1
    description: a
    scope:
      - "a/**"
      - "b/**"
  # about A-2
  - id: A-2
    description: b
  - id: A-3
    description: c
`
	want := `rules:
  - id: A-1
    description: a
    scope:
      - "a/**"
      - "b/**"

  # about A-2
  - id: A-2
    description: b

  - id: A-3
    description: c
`
	got, err := Source(src).Format()
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := Source(got).Format(); err != nil || string(twice) != want {
		t.Fatalf("not idempotent:\n%s\n%v", twice, err)
	}
}

// kind and subject go right after id, then the fields of rule.Rule in order; a comment stays with its key.
func TestOrderKindSubject(t *testing.T) {
	src := `rules:
  - check: tests/a_test.go
    deterministic: true
    scope: ["**"]
    description: d
    # what it applies to
    subject: the code
    kind: layout
    id: A-1
`
	want := `rules:
  - id: A-1
    kind: layout
    # what it applies to
    subject: the code
    description: d
    scope: ["**"]
    deterministic: true
    check: tests/a_test.go
`
	got, err := Source(src).Format()
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
}
