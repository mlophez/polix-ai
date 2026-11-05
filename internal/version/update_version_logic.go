package version

import (
	"errors"
	"fmt"
)

// UpdateVersionOutput is the decision of the logic of UpdateVersion, which the shell carries out through the
// port.
type UpdateVersionOutput struct {
	Failure   string // why the update fails before installing anything; empty when it does not
	Message   string // what to tell when nothing is installed; empty for nothing to say
	Install   string // the version to download and install over the running binary; empty for none
	Installed string // what to tell once Install is installed
	Remember  bool   // once Install is installed, record it as the latest release checked
	Refresh   bool   // refresh the plugin with the binary left when the update does not fail
	// RecordFailure records a failure of asking the latest release or of the install in the cache, so the session
	// start hook waits RetryAfter before trying again.
	RecordFailure bool
	// RefreshAfterFailure refreshes the plugin also after an install that failed, unless its replace did: then
	// another process may have updated the binary, and this one must not rewrite the plugin of the new one.
	RefreshAfterFailure bool
}

// UpdateVersion decides what polix update does for the polix of version running at exe, asked for want (empty for
// the latest release), with --check (only report) and --hook (the session start hook), and disabled by
// POLIX_NO_UPDATE. due tells whether the hook checks now (Cache.ShouldCheck); latest is the latest release, asked
// when want is empty and the update goes on, and failed the error of asking it.
func UpdateVersion(running, want, exe string, check, hook, disabled, due bool, latest string,
	failed error) UpdateVersionOutput {
	out := updateVersionDecide(running, want, exe, check, hook, disabled, due, latest, failed)
	out.RecordFailure = hook && !disabled
	out.RefreshAfterFailure = hook && !check && !disabled
	return out
}

// Record reports whether err, the failure of a step, is recorded in the cache.
func (o UpdateVersionOutput) Record(err error) bool { return err != nil && o.RecordFailure }

// RefreshAfter reports whether the plugin is refreshed after the install ended with err (nil without install).
func (o UpdateVersionOutput) RefreshAfter(err error) bool {
	if err == nil {
		return o.Refresh
	}
	return o.Refresh && o.RefreshAfterFailure && !errors.Is(err, ErrReplace)
}

// updateVersionDecide is the decision of UpdateVersion but for its policy on failures.
func updateVersionDecide(running, want, exe string, check, hook, disabled, due bool, latest string,
	failed error) UpdateVersionOutput {
	switch {
	case disabled && hook:
		return UpdateVersionOutput{}
	case disabled:
		return UpdateVersionOutput{Failure: "updates are disabled by POLIX_NO_UPDATE"}
	case hook && !due:
		return UpdateVersionOutput{Refresh: !check}
	}
	// A failure leaves the plugin to the session start hook, which refreshes the one already installed anyway.
	refreshFailed := !check && hook
	target := want
	if want == "" {
		switch {
		case failed != nil:
			return UpdateVersionOutput{Failure: failed.Error(), Refresh: refreshFailed}
		case !Released(running) && check:
			return UpdateVersionOutput{
				Message: fmt.Sprintf("polix %s is a development build; the latest release is %s", running, latest)}
		case !Released(running):
			return UpdateVersionOutput{Failure: fmt.Sprintf("polix %s is a development build: run polix update "+
				"--version %s to replace it", running, latest), Refresh: refreshFailed}
		case Compare(running, latest) >= 0 && hook:
			return UpdateVersionOutput{Refresh: !check}
		case Compare(running, latest) >= 0:
			return UpdateVersionOutput{Message: fmt.Sprintf("polix %s is up to date", running), Refresh: !check}
		}
		target = latest
	}
	if check && want != "" {
		switch c := Compare(want, running); {
		case !Released(running):
			return UpdateVersionOutput{Message: fmt.Sprintf("polix %s is a development build: run polix update "+
				"--version %s to install %s", running, want, want)}
		case c > 0:
			return UpdateVersionOutput{Message: fmt.Sprintf("polix %s is newer than this binary (%s): run polix "+
				"update --version %s", want, running, want)}
		case c == 0:
			return UpdateVersionOutput{Message: fmt.Sprintf("polix %s is already this binary", want)}
		default:
			return UpdateVersionOutput{Message: fmt.Sprintf("polix %s is older than this binary (%s): installing "+
				"it would be a downgrade", want, running)}
		}
	}
	if check {
		return UpdateVersionOutput{Message: fmt.Sprintf("polix %s can be updated to %s: run polix update", running,
			target)}
	}
	installed := fmt.Sprintf("polix updated from %s to %s (%s)", running, target, exe)
	if hook {
		installed = fmt.Sprintf("polix se ha actualizado de %s a %s.", running, target)
	}
	return UpdateVersionOutput{Install: target, Installed: installed, Remember: want == "", Refresh: true}
}
