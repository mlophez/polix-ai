package version

import "errors"

// Update is an update of polix as it is asked: the running binary, the version to install and how the caller
// wants it carried out. It is built with a literal followed by Valid.
type Update struct {
	Running  string // version of the running polix
	Want     string // the version to install, without its leading v; empty for the latest release
	Exe      string // real path of the running binary, which the update replaces
	Check    bool   // only report what the update would do
	Hook     bool   // the session start hook: quiet, and it never fails
	Disabled bool   // POLIX_NO_UPDATE is set: updates are disabled
}

// Valid tells why u is not an update: the version and the path of the running binary are known.
func (u Update) Valid() error {
	switch {
	case u.Running == "":
		return errors.New("an update needs the version of the running polix")
	case u.Exe == "":
		return errors.New("an update needs the path of the running binary")
	}
	return nil
}

// Latest reports whether the update goes to the latest release, since no version is asked.
func (u Update) Latest() bool { return u.Want == "" }

// Silent reports whether the update does nothing and says nothing: disabled, from the session start hook.
func (u Update) Silent() bool { return u.Disabled && u.Hook }

// Records reports whether a failure of the update is recorded in the cache, so the session start hook waits
// RetryAfter before trying again: only the hook, when updates are not disabled.
func (u Update) Records() bool { return u.Hook && !u.Disabled }

// Installs reports whether the update may install a binary: neither disabled nor asked only to report.
func (u Update) Installs() bool { return !u.Check && !u.Disabled }
