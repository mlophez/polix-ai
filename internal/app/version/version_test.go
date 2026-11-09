package version

import "testing"

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b Version
		want int
	}{
		{Version{Minor: 2}, Version{Minor: 2}, 0},
		{Version{Minor: 2}, Version{Minor: 10}, -1},
		{Version{Major: 1}, Version{Minor: 99, Patch: 99}, 1},
		{Version{Minor: 2, Pre: "rc1"}, Version{Minor: 2}, -1},
		{Version{Minor: 2}, Version{Minor: 2, Pre: "rc1"}, 1},
		{Version{Minor: 2, Pre: "rc1"}, Version{Minor: 2, Pre: "rc2"}, -1},
		{Version{Minor: 2, Pre: "rc2"}, Version{Minor: 2, Pre: "rc1"}, 1},
	}
	for _, c := range cases {
		if got := c.a.Compare(c.b); got != c.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestVersionIsRelease(t *testing.T) {
	for v, want := range map[Version]bool{{Minor: 2}: true, {Major: 1, Pre: "rc1"}: true, {}: false} {
		if got := v.IsRelease(); got != want {
			t.Errorf("%s.IsRelease() = %v, want %v", v, got, want)
		}
	}
}

func TestVersionValid(t *testing.T) {
	for v, ok := range map[Version]bool{{Major: 1}: true, {Major: 1, Pre: "rc.1"}: true, {Major: -1}: false,
		{Patch: -2}: false, {Major: 1, Pre: "rc 1"}: false} {
		if err := v.Valid(); (err == nil) != ok {
			t.Errorf("%+v.Valid() = %v, want ok %v", v, err, ok)
		}
	}
}

func TestVersionString(t *testing.T) {
	for v, want := range map[Version]string{{Major: 1, Minor: 2, Patch: 3}: "1.2.3", {Minor: 2, Pre: "rc1"}: "0.2.0-rc1"} {
		if got := v.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
