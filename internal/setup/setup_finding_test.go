package setup

import "testing"

func TestFinding(t *testing.T) {
	bad := Finding{Level: "info", Area: "x", Message: "y"}
	if bad.Valid() == nil || found("info", "x", "y") != nil || found(LevelOK, "", "y") != nil {
		t.Fatal("an unknown level, or no area, is not a finding")
	}
	f := append(found(LevelOK, "git", "repository at %s", "/r"), found(LevelWarn, "hooks", "old")...)
	if len(f) != 2 || f[0].Message != "repository at /r" || f[0].Problem() || !f[1].Problem() || Failed(f) {
		t.Fatalf("%+v", f)
	}
	if !Failed(found(LevelFail, "rules", "x")) {
		t.Fatal("failed")
	}
	i := Inspection{Setup: Setup{Version: "1"}}
	if i.Valid() != nil {
		t.Fatal("inspection")
	}
}
