package version

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// pattern matches a version as polix writes it: three numbers, maybe a leading v and a -prerelease.
var pattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.]+))?$`)

// prePattern matches a pre-release.
var prePattern = regexp.MustCompile(`^[0-9A-Za-z.]+$`)

// Parse reads a version like 1.2.3, v1.2.3 or 1.2.3-rc1. Anything else, a development build ("dev") among them,
// is an error with the zero Version, which compares as 0.0.0.
func Parse(s string) (Version, error) {
	m := pattern.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("%q is not a version like 1.2.3", s)
	}
	var n [3]int
	for i := range n {
		var err error
		if n[i], err = strconv.Atoi(m[i+1]); err != nil {
			return Version{}, fmt.Errorf("%q is not a version like 1.2.3: %w", s, err)
		}
	}
	v := Version{Major: n[0], Minor: n[1], Patch: n[2], Pre: m[4]}
	if err := v.Valid(); err != nil {
		return Version{}, err
	}
	return v, nil
}

// Released reports whether s is the version of a release, with the semantics of internal/platform/semver: a
// development build ("dev"), an empty text and 0.0.0 are not.
func Released(s string) bool { return semver.IsRelease(s) }

// Compare compares the versions a and b as text: -1, 0 or 1, with the semantics of internal/platform/semver: a
// missing number counts as 0 (0.3 is 0.3.0) and a pre-release is lower than its release.
func Compare(a, b string) int { return semver.Compare(a, b) }
