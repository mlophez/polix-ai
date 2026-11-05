package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"gopkg.in/yaml.v3"
)

// TestRulesChecks guards the link between the rules and their tests: each deterministic rule of
// ai/rules/architecture.yaml with a check names a file that exists and declares func Test<name>, where <name> is
// the id without separators (GOARCH-4 is TestGOARCH4), the test the runner of rules.run in polix.yaml runs.
func TestRulesChecks(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(*root, "ai", "rules", "architecture.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Rules []struct {
			ID            string `yaml:"id"`
			Deterministic bool   `yaml:"deterministic"`
			Check         string `yaml:"check"`
		} `yaml:"rules"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, r := range doc.Rules {
		if !r.Deterministic || r.Check == "" {
			continue
		}
		checked++
		name := "Test" + strings.Map(func(c rune) rune {
			if unicode.IsLetter(c) || unicode.IsDigit(c) {
				return c
			}
			return -1
		}, r.ID)
		f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(*root, filepath.FromSlash(r.Check)), nil, 0)
		if err != nil {
			t.Errorf("%s: check %s: %v", r.ID, r.Check, err)
			continue
		}
		if obj := f.Scope.Lookup(name); obj == nil || obj.Kind != ast.Fun {
			t.Errorf("%s: check %s declares no func %s", r.ID, r.Check, name)
		}
	}
	if checked == 0 {
		t.Fatal("no deterministic rule has a check")
	}
}
