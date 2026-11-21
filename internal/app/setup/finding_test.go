package setup

import "testing"

func TestFinding(t *testing.T) {
	bad := Finding{Level: "info", Area: "x", Message: "y"}
	noArea := Finding{Level: LevelOK, Message: "y"}
	if bad.Valid() == nil || noArea.Valid() == nil {
		t.Fatal("an unknown level, or no area, is not a finding")
	}
	ok, warn := Finding{Level: LevelOK, Area: "git", Message: "m"}, Finding{Level: LevelWarn, Area: "hooks", Message: "old"}
	fail := Finding{Level: LevelFail, Area: "rules", Message: "x"}
	if ok.Problem() || !warn.Problem() || warn.Failure() || !fail.Failure() {
		t.Fatal("problem and failure")
	}
	i := Inspection{Setup: Setup{Version: "1"}}
	if i.Valid() != nil {
		t.Fatal("inspection")
	}
}

// A git that lists the hooks of its configuration was asked for them: the inspection holds what it listed.
func TestInspectionGitHooks(t *testing.T) {
	asked := Inspection{Setup: Setup{Version: "1"}, ListsHooks: true, GitHooks: map[string][]string{}}
	old := Inspection{Setup: Setup{Version: "1"}, GitHooks: map[string][]string{}}
	if asked.Valid() != nil || old.Valid() != nil {
		t.Fatal("a git asked for its hooks")
	}
	if (Inspection{Setup: Setup{Version: "1"}, ListsHooks: true}).Valid() == nil {
		t.Fatal("a git that lists hooks without the hooks it listed")
	}
}

// Each known area belongs to a section, git to Machine when it fails, and an unknown area is not a finding.
func TestFindingSection(t *testing.T) {
	want := map[string]string{
		"binary": SectionMachine, "version": SectionMachine, "path": SectionMachine,
		"git": SectionRepository, "repo": SectionRepository, "workflow": SectionRepository,
		"requirements": SectionRepository, "rules": SectionRepository, "githooks": SectionRepository,
		"state": SectionRepository, "hooks": SectionAgents, "memory": SectionAgents, "plugin": SectionAgents,
		"codex": SectionAgents,
	}
	if len(want) != 14 {
		t.Fatal("fourteen known areas")
	}
	for area, section := range want {
		f := Finding{Level: LevelOK, Area: area, Message: "m"}
		if f.Valid() != nil || f.Section() != section {
			t.Errorf("%s: section %q, want %q (%v)", area, f.Section(), section, f.Valid())
		}
	}
	if got := (Finding{Level: LevelFail, Area: "git", Message: "m"}).Section(); got != SectionMachine {
		t.Fatalf("git that fails: %q", got)
	}
	if got := (Finding{Level: LevelWarn, Area: "git", Message: "m"}).Section(); got != SectionRepository {
		t.Fatalf("git that warns: %q", got)
	}
	unknown := Finding{Level: LevelOK, Area: "nowhere", Message: "m"}
	if unknown.Valid() == nil || unknown.Section() != "" {
		t.Fatal("an unknown area is not valid")
	}
}

// The text of a finding is its message, followed by `: ` and its fix when it has one.
func TestFindingText(t *testing.T) {
	plain := Finding{Level: LevelOK, Area: "git", Message: "repository at /r"}
	fixed := Finding{Level: LevelWarn, Area: "hooks", Message: "Stop does not run polix hook claude", Fix: "run polix install"}
	if plain.Text() != "repository at /r" {
		t.Fatal(plain.Text())
	}
	if fixed.Text() != "Stop does not run polix hook claude: run polix install" {
		t.Fatal(fixed.Text())
	}
	if fixed.Valid() != nil {
		t.Fatal("a fix is valid")
	}
}
