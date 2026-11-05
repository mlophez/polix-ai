package version

import "testing"

func TestShowVersion(t *testing.T) {
	for _, v := range []string{"dev", "0.2.0", "1.0.0-rc1"} {
		if got := ShowVersion(v); got.Version != v {
			t.Errorf("ShowVersion(%q) = %+v", v, got)
		}
	}
}
