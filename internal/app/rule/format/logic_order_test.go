package format

import (
	"testing"
)

// rules goes first, the keys of a rule follow the schema with the unknown ones after, and the rules never move; a
// comment above a key moves with it, a flow scope on two lines ends on one and a folded description keeps its text.
func TestFormatData(t *testing.T) {
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
    reason: why
    x-note: kept last

  - id: A-1
    description: d
x-owner: platform
`
	got, err := formatData([]byte(src))
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
}

// Three rules written without blank lines come out with one blank line between each two, above the comments of a
// rule, and none after rules:; the scope of a rule in block style gets no blank line.
func TestFormatDataSpacesTheRules(t *testing.T) {
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
	got, err := formatData([]byte(src))
	if err != nil || string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s\n%v", got, want, err)
	}
	if twice, err := formatData(got); err != nil || string(twice) != want {
		t.Fatalf("not idempotent:\n%s\n%v", twice, err)
	}
}
