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

// ruleCheck is a rule of a file of ai/rules/, with what TestRulesChecks reads of it.
type ruleCheck struct {
	ID            string `yaml:"id"`
	Deterministic bool   `yaml:"deterministic"`
	Check         string `yaml:"check"`
}

// TestRulesChecks guards the link between the rules and their tests: each deterministic rule with a check of every
// file of ai/rules/ but retired.yaml, whose rules never apply, names a file that exists and declares func
// Test<name>, where <name> is the id without separators (GOARCH-4 is TestGOARCH4, GO-4 is TestGO4), the test the
// runner of rules.run in polix.yaml runs.
func TestRulesChecks(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(*root, "ai", "rules", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var rules []ruleCheck
	for _, file := range files {
		if filepath.Base(file) == "retired.yaml" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Rules []ruleCheck `yaml:"rules"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", filepath.Base(file), err)
		}
		rules = append(rules, doc.Rules...)
	}
	checked := 0
	for _, r := range rules {
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
