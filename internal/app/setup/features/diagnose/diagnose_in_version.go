package diagnose

import (
	"fmt"
	"regexp"
	"strconv"

	"github.com/mlophez/polix-ai/internal/app/version"
)

// versionText is the version of the running polix as the request carries it, a text like 1.2.3, v1.2.3 or
// 1.2.3-rc1, which the shell translates into a version.Version.
type versionText string

// versionPattern matches a version as polix writes it: three numbers, maybe a leading v and a -prerelease.
var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.]+))?$`)

// parse translates the text into a version.Version, validated with the domain. Anything else, a development build
// ("dev") among them, is an error with the zero Version, which compares as 0.0.0.
func (s versionText) parse() (version.Version, error) {
	m := versionPattern.FindStringSubmatch(string(s))
	if m == nil {
		return version.Version{}, fmt.Errorf("%q is not a version like 1.2.3", string(s))
	}
	var n [3]int
	for i := range n {
		var err error
		if n[i], err = strconv.Atoi(m[i+1]); err != nil {
			return version.Version{}, fmt.Errorf("%q is not a version like 1.2.3: %w", string(s), err)
		}
	}
	v := version.Version{Major: n[0], Minor: n[1], Patch: n[2], Pre: m[4]}
	if err := v.Valid(); err != nil {
		return version.Version{}, err
	}
	return v, nil
}
