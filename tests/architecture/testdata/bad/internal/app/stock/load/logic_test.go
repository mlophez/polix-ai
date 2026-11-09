package load

import "testing"

func TestDecide(t *testing.T) {
	if out := Decide(1, nil); out.Loaded.Count != 1 {
		t.Fatal(out)
	}
}
