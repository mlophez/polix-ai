package workflow

import (
	"strings"
	"testing"
)

// AC10: a measure condition is a non-negative integer; a map of the old vocabulary is reported with the number to
// write instead.
func TestCondMeasured(t *testing.T) {
	m, err := Cond{"risk_score": 150}.Measured("risk_score")
	if err != nil || m.AtLeast != 150 {
		t.Fatalf("Measured = %+v, %v", m, err)
	}
	if m, err := (Cond{"risk_score": 0}).Measured("risk_score"); err != nil || m.AtLeast != 0 {
		t.Fatalf("Measured of 0 = %+v, %v", m, err)
	}
	for _, v := range []any{-1, "150", 2.5, map[string]any{}, Cond{"at_most": 2}} {
		_, err := Cond{"risk_score": v}.Measured("risk_score")
		if err == nil || !strings.Contains(err.Error(), "needs a non-negative integer") {
			t.Errorf("Measured(%v): %v", v, err)
		}
	}
	_, err = Cond{"risk_score": map[string]any{"at_least": 150}}.Measured("risk_score")
	if err == nil || !strings.Contains(err.Error(), "use 150 instead of { at_least: 150 }") {
		t.Fatalf("the old map: %v", err)
	}
	if (Measure{AtLeast: -1}).Valid() == nil || (Measure{AtLeast: 3}).Valid() != nil {
		t.Fatal("Measure.Valid")
	}
}

func TestNestedAndStrings(t *testing.T) {
	if c, ok := (Cond{"not": map[string]any{"actor": "session"}}).Nested("not"); !ok || c["actor"] != "session" {
		t.Fatal("a map is a Cond")
	}
	if _, ok := (Cond{"not": "x"}).Nested("not"); ok {
		t.Fatal("a string is not a Cond")
	}
	if l, err := (Cond{"actor": []any{"a", "b"}}).Strings("actor"); err != nil || len(l) != 2 {
		t.Fatal(l, err)
	}
	if _, err := (Cond{"actor": []any{"a", 1}}).Strings("actor"); err == nil {
		t.Fatal("a number in the list")
	}
	if _, err := (Cond{"actor": 3}).Strings("actor"); err == nil {
		t.Fatal("a number")
	}
}
