package update

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/version"
)

func TestDecide(t *testing.T) {
	offline := errors.New("dial tcp: connection refused")
	cases := []struct {
		name                       string
		running, want              string
		check, hook, disabled, due bool
		latest                     string
		failed                     error
		wantOut                    Output
		failure, message           string // substrings, when the field is not compared whole
	}{
		{name: "disabled", running: "0.1.0", disabled: true,
			failure: "updates are disabled by POLIX_NO_UPDATE"},
		{name: "disabled hook", running: "0.1.0", hook: true, disabled: true, wantOut: Output{}},
		{name: "hook not due", running: "0.1.0", hook: true, wantOut: Output{Refresh: true,
			RecordFailure: true, RefreshAfterFailure: true}},
		{name: "hook check not due", running: "0.1.0", hook: true, check: true,
			wantOut: Output{RecordFailure: true}},
		{name: "latest failed", running: "0.1.0", due: true, failed: offline,
			wantOut: Output{Failure: offline.Error()}},
		{name: "latest failed hook", running: "0.1.0", hook: true, due: true, failed: offline,
			wantOut: Output{Failure: offline.Error(), Refresh: true, RecordFailure: true,
				RefreshAfterFailure: true}},
		{name: "dev check", running: "dev", check: true, due: true, latest: "0.2.0",
			wantOut: Output{Message: "polix dev is a development build; the latest release is 0.2.0"}},
		{name: "dev", running: "dev", due: true, latest: "0.2.0", wantOut: Output{
			Failure: "polix dev is a development build: run polix update --version 0.2.0 to replace it"}},
		{name: "up to date", running: "0.2.0", due: true, latest: "0.2.0",
			wantOut: Output{Message: "polix 0.2.0 is up to date", Refresh: true}},
		{name: "up to date check", running: "0.2.0", check: true, due: true, latest: "0.2.0",
			wantOut: Output{Message: "polix 0.2.0 is up to date"}},
		{name: "up to date hook", running: "0.3.0", hook: true, due: true, latest: "0.2.0",
			wantOut: Output{Refresh: true, RecordFailure: true, RefreshAfterFailure: true}},
		{name: "check", running: "0.1.0", check: true, due: true, latest: "0.2.0",
			wantOut: Output{Message: "polix 0.1.0 can be updated to 0.2.0: run polix update"}},
		{name: "update", running: "0.1.0", due: true, latest: "0.2.0", wantOut: Output{
			Install: "0.2.0", Installed: "polix updated from 0.1.0 to 0.2.0 (/bin/polix)", Remember: true, Refresh: true}},
		{name: "update hook", running: "0.1.0", hook: true, due: true, latest: "0.2.0", wantOut: Output{
			Install: "0.2.0", Installed: "polix se ha actualizado de 0.1.0 a 0.2.0.", Remember: true, Refresh: true,
			RecordFailure: true, RefreshAfterFailure: true}},
		{name: "a version, older ones included", running: "0.2.0", want: "0.1.0", due: true,
			wantOut: Output{Install: "0.1.0",
				Installed: "polix updated from 0.2.0 to 0.1.0 (/bin/polix)", Refresh: true}},
		{name: "a version over a dev build", running: "dev", want: "0.2.0", due: true,
			wantOut: Output{Install: "0.2.0",
				Installed: "polix updated from dev to 0.2.0 (/bin/polix)", Refresh: true}},
		{name: "check newer version", running: "0.2.0", want: "0.3.0", check: true, due: true,
			message: "is newer than this binary"},
		{name: "check same version", running: "0.2.0", want: "0.2.0", check: true, due: true,
			message: "is already this binary"},
		{name: "check older version", running: "0.2.0", want: "0.1.0", check: true, due: true,
			message: "would be a downgrade"},
		{name: "check version on dev", running: "dev", want: "0.1.0", check: true, due: true,
			message: "polix dev is a development build: run polix update --version 0.1.0 to install 0.1.0"},
	}
	for _, c := range cases {
		got := Decide(c.running, c.want, "/bin/polix", c.check, c.hook, c.disabled, c.due, c.latest, c.failed)
		switch {
		case c.failure != "" || c.message != "":
			if !strings.Contains(got.Failure, c.failure) || !strings.Contains(got.Message, c.message) ||
				got.Install != "" || got.Refresh {
				t.Errorf("%s: %+v", c.name, got)
			}
		case got != c.wantOut:
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.wantOut)
		}
	}
}

// The policy on failures: the hook records them and refreshes after any of them but a failed replace; a person
// neither records them nor refreshes after one.
func TestOutputFailures(t *testing.T) {
	broken := errors.New("checksum does not match")
	replace := fmt.Errorf("%w: permission denied", version.ErrReplace)
	hook := Decide("0.1.0", "", "/bin/polix", false, true, false, true, "0.2.0", nil)
	if !hook.Record(broken) || hook.Record(nil) || !hook.RefreshAfter(nil) || !hook.RefreshAfter(broken) ||
		hook.RefreshAfter(replace) {
		t.Fatalf("hook: %+v", hook)
	}
	person := Decide("0.1.0", "", "/bin/polix", false, false, false, true, "0.2.0", nil)
	if person.Record(broken) || !person.RefreshAfter(nil) || person.RefreshAfter(broken) {
		t.Fatalf("person: %+v", person)
	}
	disabled := Decide("0.1.0", "", "/bin/polix", false, true, true, true, "", nil)
	if disabled.Record(broken) || disabled.RefreshAfter(nil) {
		t.Fatalf("disabled: %+v", disabled)
	}
}
