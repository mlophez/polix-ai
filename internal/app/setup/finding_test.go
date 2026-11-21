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
