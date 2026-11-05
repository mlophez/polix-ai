package build

import (
	"runtime/debug"
	"testing"
)

func TestCurrent(t *testing.T) {
	info := func(v string) *debug.BuildInfo { return &debug.BuildInfo{Main: debug.Module{Version: v}} }
	cases := []struct {
		name, version string
		info          *debug.BuildInfo
		ok            bool
		want          string
	}{
		{"set by the linker", "0.3.0", info("v0.4.0"), true, "0.3.0"},
		{"tagged module", "dev", info("v0.4.0"), true, "0.4.0"},
		{"tagged prerelease", "dev", info("v0.4.0-rc.1"), true, "0.4.0-rc.1"},
		{"pseudo-version", "dev", info("v0.4.1-0.20260101120000-abcdef123456"), true, "dev"},
		{"local build", "dev", info("(devel)"), true, "dev"},
		{"no build info", "dev", nil, false, "dev"},
	}
	for _, c := range cases {
		if got := current(c.version, c.info, c.ok); got != c.want {
			t.Errorf("%s: current = %q, want %q", c.name, got, c.want)
		}
	}
	if Current() == "" {
		t.Fatal("Current is never empty")
	}
}
