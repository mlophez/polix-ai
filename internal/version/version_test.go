package version

import "testing"

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "0.2.0", 0},
		{"v0.2.0", "0.2.0", 0},
		{"0.2.0", "0.10.0", -1},
		{"1.0.0", "0.99.99", 1},
		{"0.2.0-rc1", "0.2.0", -1},
		{"0.2.0", "0.2.0-rc1", 1},
		{"0.2.0-rc1", "0.2.0-rc2", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestIsRelease(t *testing.T) {
	for v, want := range map[string]bool{"0.2.0": true, "1.0.0-rc1": true, "dev": false, "": false, "0.0.0": false} {
		if got := IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestValid(t *testing.T) {
	for v, want := range map[string]bool{"0.2.0": true, "v1.2.3": true, "1.0.0-rc.1": true, "0.2": false, "dev": false, "": false, "1.2.3.4": false} {
		if got := Valid(v); got != want {
			t.Errorf("Valid(%q) = %v, want %v", v, got, want)
		}
	}
}
