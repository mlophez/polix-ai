// Package version is the domain of the versions of polix and of its update: a version as polix writes it and the
// cache of the checks of the latest release. Its use cases are ShowVersion (internal/app/version/show, polix
// version) and UpdateVersion (internal/app/version/update, polix update).
package version

import (
	"errors"
	"fmt"
	"strconv"
)

// Version is a version of polix: major.minor.patch with an optional pre-release (1.2.3, 1.2.3-rc1).
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string // the pre-release, without its dash; empty for a release
}

// ErrReplace marks an update whose binary could not take the place of the running one: after it, another process
// may have updated the binary, so this one must not refresh the plugin with its own.
var ErrReplace = errors.New("cannot replace the binary")

// Valid tells why v is not a version: a negative number, or a pre-release with other characters than letters,
// digits and dots.
func (v Version) Valid() error {
	if v.Major < 0 || v.Minor < 0 || v.Patch < 0 {
		return fmt.Errorf("version %s has a negative number", v)
	}
	if v.Pre != "" && !prePattern.MatchString(v.Pre) {
		return fmt.Errorf("version %s has an invalid pre-release", v)
	}
	return nil
}

// Compare compares v with o: -1, 0 or 1. A pre-release is lower than its release; pre-releases compare as text.
func (v Version) Compare(o Version) int {
	for _, d := range [][2]int{{v.Major, o.Major}, {v.Minor, o.Minor}, {v.Patch, o.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.Pre == o.Pre:
		return 0
	case v.Pre == "":
		return 1
	case o.Pre == "":
		return -1
	case v.Pre < o.Pre:
		return -1
	}
	return 1
}

// IsRelease reports whether v is a released version: 0.0.0, the version of no release, is not.
func (v Version) IsRelease() bool {
	return v.Major != 0 || v.Minor != 0 || v.Patch != 0
}

// String writes v as polix does, without a leading v: 1.2.3 or 1.2.3-rc1.
func (v Version) String() string {
	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}
