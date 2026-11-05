package rule

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The shell is tested with a fake of the port: no disk, git nor processes.

// fakeCheckRulePort answers the port of CheckRule from memory.
type fakeCheckRulePort struct {
	rules      []Rule
	changed    []string
	baseErr    error            // error of reading the base branches of polix.yaml
	noBase     bool             // no base branch exists
	fails      map[string]error // error of the run of each script that fails
	candidates *[]string        // where Changed records the bases it was asked for
}

func (f fakeCheckRulePort) LoadRules(_ context.Context) ([]Rule, error) { return f.rules, nil }

func (f fakeCheckRulePort) BaseBranches(_ context.Context) ([]string, error) {
	return []string{"main"}, f.baseErr
}

func (f fakeCheckRulePort) Changed(_ context.Context, candidates []string) ([]string, error) {
	if f.candidates != nil {
		*f.candidates = candidates
	}
	if f.noBase {
		return nil, errors.New("no base branch exists")
	}
	return f.changed, nil
}

func (f fakeCheckRulePort) RunScript(_ context.Context, script string) (string, error) {
	return "ran " + script, f.fails[script]
}

// shellRules are two rules with a check and one without.
var shellRules = []Rule{
	{ID: "go", Description: "gofmt", Scope: []string{"**/*.go"}, Deterministic: true, Check: "ai/checks/gofmt.sh"},
	{ID: "sh", Description: "shfmt", Scope: []string{"**/*.sh"}, Deterministic: true, Check: "ai/checks/shfmt.sh"},
	{ID: "prose", Description: "no check"},
}

func TestCheckRuleShellRunsTheChecksAndReportsTheFailures(t *testing.T) {
	port := fakeCheckRulePort{rules: shellRules, fails: map[string]error{"ai/checks/shfmt.sh": errors.New("exit 1")}}
	res, err := checkRule(context.Background(), port, CheckRuleRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Ran != 2 || len(res.Failed) != 1 || res.Failed[0].ID != "sh" {
		t.Fatalf("got %+v", res)
	}
	if want := "ran ai/checks/shfmt.sh\nexit 1"; res.Failed[0].Output != want {
		t.Fatalf("output %q, want %q", res.Failed[0].Output, want)
	}
}

func TestCheckRuleShellOnlyChanged(t *testing.T) {
	port := fakeCheckRulePort{rules: shellRules, changed: []string{"install.sh"}}
	res, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true})
	if err != nil || res.Ran != 1 {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestCheckRuleShellAsksForABase(t *testing.T) {
	port := fakeCheckRulePort{rules: shellRules, noBase: true}
	_, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true})
	if err == nil || !strings.Contains(err.Error(), "--base") {
		t.Fatalf("got %v, want an error that asks for --base", err)
	}
}

func TestCheckRuleShellStopsWhenPolixYAMLIsWrong(t *testing.T) {
	port := fakeCheckRulePort{rules: shellRules, baseErr: errors.New("polix.yaml: bad yaml")}
	_, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true})
	if err == nil || !strings.Contains(err.Error(), "polix.yaml") {
		t.Fatalf("got %v, want the error of polix.yaml", err)
	}
}

func TestCheckRuleShellPrefersTheBaseAsked(t *testing.T) {
	var asked []string
	port := fakeCheckRulePort{rules: shellRules, candidates: &asked}
	if _, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true, Base: "develop"}); err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0] != "develop" {
		t.Fatalf("asked git for %v, want [develop]", asked)
	}
}
