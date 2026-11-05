package rule

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	got, err := Parse("ai/rules/go.yaml", []byte(`rules:
  - id: GO-1
    description: >-
      gofmt
    scope: ["**/*.go"]
    deterministic: true
    check: ai/checks/gofmt.sh
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "GO-1" || got[0].Description != "gofmt" || got[0].File != "ai/rules/go.yaml" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseRejectsAnInvalidRule(t *testing.T) {
	_, err := Parse("ai/rules/go.yaml", []byte("rules:\n  - id: GO-1\n"))
	if err == nil || !strings.Contains(err.Error(), "ai/rules/go.yaml") {
		t.Fatalf("got %v, want the error of Valid with the file", err)
	}
}
