package version

import "testing"

func TestParse(t *testing.T) {
	for s, want := range map[string]Version{
		"0.2.0":      {Minor: 2},
		"v1.2.3":     {Major: 1, Minor: 2, Patch: 3},
		"1.0.0-rc.1": {Major: 1, Pre: "rc.1"},
	} {
		if got, err := Parse(s); err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", s, got, err, want)
		}
	}
	for _, s := range []string{"0.2", "dev", "", "1.2.3.4", "1.2.3-", "x1.2.3"} {
		if got, err := Parse(s); err == nil || got != (Version{}) {
			t.Errorf("Parse(%q) = %+v, %v; want an error and the zero version", s, got, err)
		}
	}
}

func TestReleased(t *testing.T) {
	for v, want := range map[string]bool{"0.2.0": true, "1.0.0-rc1": true, "dev": false, "": false, "0.0.0": false} {
		if got := Released(v); got != want {
			t.Errorf("Released(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "v0.2.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"dev", "0.0.1", -1},
		{"0.0.1", "nightly", 1},
		{"0.3", "0.2.0", 1},
		{"1.2", "1.2.0", 0},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
