// Package version compares the versions of polix: 1.2.3, with an optional leading v and an optional -prerelease.
package version

import (
	"regexp"
	"strconv"
	"strings"
)

// valid matches a version as polix writes it: three numbers, maybe a leading v and a -prerelease.
var valid = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// Valid reports whether v is a version like 1.2.3 (or v1.2.3, 1.2.3-rc1).
func Valid(v string) bool { return valid.MatchString(v) }

// Compare compares two versions: -1, 0 or 1. A prerelease is lower than its release; prereleases compare as text.
func Compare(a, b string) int {
	an, ap := parse(a)
	bn, bp := parse(b)
	for i := range an {
		if an[i] != bn[i] {
			if an[i] < bn[i] {
				return -1
			}
			return 1
		}
	}
	switch {
	case ap == bp:
		return 0
	case ap == "":
		return 1
	case bp == "":
		return -1
	case ap < bp:
		return -1
	}
	return 1
}

func parse(v string) ([3]int, string) {
	v = strings.TrimPrefix(v, "v")
	v, pre, _ := strings.Cut(v, "-")
	var n [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		n[i], _ = strconv.Atoi(p)
	}
	return n, pre
}

// IsRelease reports whether v is a released version (not a development build).
func IsRelease(v string) bool {
	n, _ := parse(v)
	return v != "" && v != "dev" && n != [3]int{}
}
