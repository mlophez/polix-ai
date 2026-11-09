package update

import (
	"errors"
	"fmt"

	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Output is the decision of the logic of UpdateVersion, which the shell carries out through the
// port.
type Output struct {
	Failure   string // why the update fails before installing anything; empty when it does not
	Message   string // what to tell when nothing is installed; empty for nothing to say
	Install   string // the version to download and install over the running binary; empty for none
	Installed string // what to tell once Install is installed
	Remember  bool   // once Install is installed, record it as the latest release checked
	Refresh   bool   // refresh the plugin with the binary left when the update does not fail
	// RecordFailure records a failure of asking the latest release or of the install in the cache, so the session
	// start hook waits version.RetryAfter before trying again.
	RecordFailure bool
	// RefreshAfterFailure refreshes the plugin also after an install that failed, unless its replace did: then
	// another process may have updated the binary, and this one must not rewrite the plugin of the new one.
	RefreshAfterFailure bool
}

// Decide decides what polix update does for the polix of version running at exe, asked for want (empty for the
// latest release), with --check (only report) and --hook (the session start hook), and disabled by POLIX_NO_UPDATE.
// due tells whether the hook checks now (version.Cache.ShouldCheck); latest is the latest release, asked when want
// is empty and the update goes on, and failed the error of asking it.
func Decide(running, want, exe string, check, hook, disabled, due bool, latest string,
	failed error) Output {
	out := decision(running, want, exe, check, hook, disabled, due, latest, failed)
	out.RecordFailure = hook && !disabled
	out.RefreshAfterFailure = hook && !check && !disabled
	return out
}

// Record reports whether err, the failure of a step, is recorded in the cache.
func (o Output) Record(err error) bool { return err != nil && o.RecordFailure }

// RefreshAfter reports whether the plugin is refreshed after the install ended with err (nil without install).
func (o Output) RefreshAfter(err error) bool {
	if err == nil {
		return o.Refresh
	}
	return o.Refresh && o.RefreshAfterFailure && !errors.Is(err, version.ErrReplace)
}

// decision is the decision of Decide but for its policy on failures. The versions are text, compared with the
// semantics of internal/platform/semver: a development build ("dev") is not a release, and a missing number counts
// as 0.
func decision(running, want, exe string, check, hook, disabled, due bool, latest string,
	failed error) Output {
	switch {
	case disabled && hook:
		return Output{}
	case disabled:
		return Output{Failure: "updates are disabled by POLIX_NO_UPDATE"}
	case hook && !due:
		return Output{Refresh: !check}
	}
	target := want
	if want == "" {
		if out, done := towardLatest(running, latest, check, hook, failed); done {
			return out
		}
		target = latest
	}
	if check && want != "" {
		return compared(running, want)
	}
	return installing(running, target, exe, check, hook, want == "")
}

// towardLatest decides an update to the latest release, latest, asked with the error failed: whether it ends
// before installing, and then how; else the update goes on to install latest.
func towardLatest(running, latest string, check, hook bool, failed error) (Output, bool) {
	// A failure leaves the plugin to the session start hook, which refreshes the one already installed anyway.
	refreshFailed := !check && hook
	switch {
	case failed != nil:
		return Output{Failure: failed.Error(), Refresh: refreshFailed}, true
	case !semver.IsRelease(running) && check:
		return Output{
			Message: fmt.Sprintf("polix %s is a development build; the latest release is %s", running, latest)}, true
	case !semver.IsRelease(running):
		return Output{Failure: fmt.Sprintf("polix %s is a development build: run polix update "+
			"--version %s to replace it", running, latest), Refresh: refreshFailed}, true
	case semver.Compare(running, latest) >= 0 && hook:
		return Output{Refresh: !check}, true
	case semver.Compare(running, latest) >= 0:
		return Output{Message: fmt.Sprintf("polix %s is up to date", running), Refresh: !check}, true
	}
	return Output{}, false
}

// compared tells, with --check, how the version want compares to the running one, without installing it.
func compared(running, want string) Output {
	switch c := semver.Compare(want, running); {
	case !semver.IsRelease(running):
		return Output{Message: fmt.Sprintf("polix %s is a development build: run polix update "+
			"--version %s to install %s", running, want, want)}
	case c > 0:
		return Output{Message: fmt.Sprintf("polix %s is newer than this binary (%s): run polix "+
			"update --version %s", want, running, want)}
	case c == 0:
		return Output{Message: fmt.Sprintf("polix %s is already this binary", want)}
	default:
		return Output{Message: fmt.Sprintf("polix %s is older than this binary (%s): installing "+
			"it would be a downgrade", want, running)}
	}
}

// installing decides to install target over the running binary at exe, or with --check only tells that it can; the
// latest release installed is remembered as the latest checked.
func installing(running, target, exe string, check, hook, latest bool) Output {
	if check {
		return Output{Message: fmt.Sprintf("polix %s can be updated to %s: run polix update", running,
			target)}
	}
	installed := fmt.Sprintf("polix updated from %s to %s (%s)", running, target, exe)
	if hook {
		installed = fmt.Sprintf("polix se ha actualizado de %s a %s.", running, target)
	}
	return Output{Install: target, Installed: installed, Remember: latest, Refresh: true}
}
