package load

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// requirementsOf decodes src, a polix.yaml, and returns its key requirements as decoded and the error of yaml.
func requirementsOf(t *testing.T, src string) (requirementsYAML, error) {
	t.Helper()
	var y workflowYAML
	err := yaml.Unmarshal([]byte(src), &y)
	return y.Requirements, err
}

// AC1: a list of program names is decoded in file order; a null item is an empty name, and left out, null or empty
// there is no requirement.
func TestRequirementsDecode(t *testing.T) {
	cases := map[string][]string{
		"version: 1\nrequirements: [go, golangci-lint]\n":    {"go", "golangci-lint"},
		"version: 1\nrequirements:\n  - just\n  - go\n":      {"just", "go"},
		"version: 1\nrequirements: [go, ~]\n":                {"go", ""},
		"version: 1\nrequirements: [\"golangci-lint\", 3]\n": {"golangci-lint", "3"},
		"version: 1\n":                   nil,
		"version: 1\nrequirements:\n":    nil,
		"version: 1\nrequirements: []\n": nil,
	}
	for src, want := range cases {
		got, err := requirementsOf(t, src)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%q: %q %v, want %q", src, got, err, want)
		}
	}
}

// AC1: a single name, a map and an item that is not a name are an error at the line of their node with the form the
// key takes, never an error of yaml.
func TestRequirementsForms(t *testing.T) {
	const form = "requirements takes a list of program names, as [go, golangci-lint]"
	cases := map[string]string{
		"version: 1\nrequirements: go\n":                           "line 2: " + form,
		"version: 1\nrequirements:\n  go: true\n":                  "line 3: " + form,
		"version: 1\nrequirements:\n  - go\n  - [golangci-lint]\n": "line 4: " + form,
		"version: 1\nrequirements:\n  - go\n  - { name: just }\n":  "line 4: " + form,
	}
	for src, want := range cases {
		if _, err := requirementsOf(t, src); err == nil || err.Error() != want {
			t.Errorf("%q:\ngot  %v\nwant %s", src, err, want)
		}
		if _, err := parse(t, src, "dev"); err == nil || err.Error() != "polix.yaml: "+want {
			t.Errorf("%q through the loader:\ngot  %v\nwant polix.yaml: %s", src, err, want)
		}
	}
}

// AC1: an empty name decodes, and the workflow reports it.
func TestRequirementsEmptyName(t *testing.T) {
	_, err := parse(t, "version: 1\nrequirements: [go, \"\"]\n", "dev")
	if err == nil || !strings.Contains(err.Error(), "polix.yaml: requirements: a program name cannot be empty") {
		t.Fatalf("an empty name: %v", err)
	}
}
