package workflow

import (
	"math"
	"testing"
)

func TestWeightValid(t *testing.T) {
	for _, w := range []Weight{{Factor: 1}, {Glob: " ", Factor: 1}, {Glob: "a", Factor: -1}, {Glob: "a", Factor: math.NaN()},
		{Glob: "a", Factor: math.Inf(1)}} {
		if w.Valid() == nil {
			t.Errorf("%+v is valid", w)
		}
	}
	if err := (Weight{Glob: "a", Factor: 0}).Valid(); err != nil {
		t.Fatal(err)
	}
}
