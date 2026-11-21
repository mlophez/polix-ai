package show

import "testing"

func TestDecide(t *testing.T) {
	for _, v := range []string{"dev", "0.2.0", "1.0.0-rc1"} {
		if got := Decide(v); got.Version != v {
			t.Errorf("Decide(%q) = %+v", v, got)
		}
	}
}
