// Package build tells the version of the running binary.
package build

import (
	"regexp"
	"runtime/debug"
	"strings"
)

// Version of the binary. Releases set it at build time with
// -ldflags "-X github.com/mlophez/polix-ai/pkg/build.Version=..."; any other build is "dev" (see Current).
var Version = "dev"

// releasedModule matches a module version of a tag, not a pseudo-version of an untagged commit.
var releasedModule = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

var pseudoVersion = regexp.MustCompile(`\d{14}-[0-9a-f]{12}`)

// Current returns the version of the binary: Version when the linker set it, else the module version of a
// tagged build (`go install ...@vX.Y.Z`) without the leading v, else "dev".
func Current() string {
	info, ok := debug.ReadBuildInfo()
	return current(Version, info, ok)
}

func current(version string, info *debug.BuildInfo, ok bool) string {
	if version != "dev" || !ok || info == nil {
		return version
	}
	if v := info.Main.Version; releasedModule.MatchString(v) && !pseudoVersion.MatchString(v) {
		return strings.TrimPrefix(v, "v")
	}
	return version
}
