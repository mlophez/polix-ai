package rule

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// The shell is tested with a fake of the port: no disk, git nor processes.

// fakeCheckRulePort answers the port of CheckRule from memory and records the methods called.
type fakeCheckRulePort struct {
	rules      []Rule
	changed    []string
	baseErr    error            // error of reading the base branches of polix.yaml
	noBase     bool             // no base branch exists
	fails      map[string]error // error of the run of each script that fails
	calls      []string         // the methods called, in order
	candidates []string         // the bases Changed was asked for
}

func (f *fakeCheckRulePort) LoadRules(_ context.Context) ([]Rule, error) {
	f.calls = append(f.calls, "LoadRules")
	return f.rules, nil
}

func (f *fakeCheckRulePort) BaseBranches(_ context.Context) ([]string, error) {
	f.calls = append(f.calls, "BaseBranches")
	return []string{"main"}, f.baseErr
}

func (f *fakeCheckRulePort) Changed(_ context.Context, candidates []string) ([]string, error) {
	f.calls = append(f.calls, "Changed")
	f.candidates = candidates
	if f.noBase {
		return nil, errors.New("no base branch exists: set base in polix.yaml or pass --base")
	}
	return f.changed, nil
}

func (f *fakeCheckRulePort) RunScript(_ context.Context, script string) (string, error) {
	f.calls = append(f.calls, "RunScript "+script)
	return "ran " + script, f.fails[script]
}

// shellRules are two rules with a check and one without.
var shellRules = []Rule{
	{ID: "go", Description: "gofmt", Scope: []string{"**/*.go"}, Deterministic: true, Check: "ai/checks/gofmt.sh"},
	{ID: "sh", Description: "shfmt", Scope: []string{"**/*.sh"}, Deterministic: true, Check: "ai/checks/shfmt.sh"},
	{ID: "prose", Description: "no check"},
}

func TestCheckRuleShellRunsTheChecksAndReportsTheFailures(t *testing.T) {
	port := &fakeCheckRulePort{rules: shellRules, fails: map[string]error{"ai/checks/shfmt.sh": errors.New("exit 1")}}
	res, err := checkRule(context.Background(), port, CheckRuleRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := CheckRuleResponse{Ran: 2, Failed: []CheckRuleFailed{
		{ID: "sh", Check: "ai/checks/shfmt.sh", Description: "shfmt", Output: "ran ai/checks/shfmt.sh\nexit 1"},
	}}
	if res.Ran != want.Ran || !slices.Equal(res.Failed, want.Failed) {
		t.Fatalf("got %+v, want %+v", res, want)
	}
	calls := []string{"LoadRules", "RunScript ai/checks/gofmt.sh", "RunScript ai/checks/shfmt.sh"}
	if !slices.Equal(port.calls, calls) {
		t.Fatalf("called %v, want %v: without --changed the shell asks nothing about the branch", port.calls, calls)
	}
}

func TestCheckRuleShellOnlyChanged(t *testing.T) {
	port := &fakeCheckRulePort{rules: shellRules, changed: []string{"install.sh"}}
	res, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true})
	if err != nil || res.Ran != 1 {
		t.Fatalf("got %+v, %v", res, err)
	}
	if !slices.Equal(port.candidates, []string{"main"}) {
		t.Fatalf("asked git for %v, want the bases of polix.yaml", port.candidates)
	}
}

func TestCheckRuleShellAsksForABase(t *testing.T) {
	port := &fakeCheckRulePort{rules: shellRules, noBase: true}
	_, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true})
	if err == nil || !strings.Contains(err.Error(), "--base") {
		t.Fatalf("got %v, want the error of the port, which asks for --base", err)
	}
}

func TestCheckRuleShellStopsWhenPolixYAMLIsWrong(t *testing.T) {
	port := &fakeCheckRulePort{rules: shellRules, baseErr: errors.New("polix.yaml: bad yaml")}
	_, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true})
	if err == nil || !strings.Contains(err.Error(), "polix.yaml") {
		t.Fatalf("got %v, want the error of polix.yaml", err)
	}
}

func TestCheckRuleShellPrefersTheBaseAsked(t *testing.T) {
	port := &fakeCheckRulePort{rules: shellRules, baseErr: errors.New("polix.yaml: bad yaml")}
	if _, err := checkRule(context.Background(), port, CheckRuleRequest{OnlyChanged: true, Base: "develop"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(port.candidates, []string{"develop"}) || slices.Contains(port.calls, "BaseBranches") {
		t.Fatalf("asked git for %v after %v: with --base, polix.yaml is not read", port.candidates, port.calls)
	}
}

func TestCheckRuleFlags(t *testing.T) {
	if err := checkRuleFlags(CheckRuleRequest{Base: "develop"}); err == nil {
		t.Fatal("--base without --changed is accepted")
	}
	if err := checkRuleFlags(CheckRuleRequest{Base: "develop", OnlyChanged: true}); err != nil {
		t.Fatal(err)
	}
}
