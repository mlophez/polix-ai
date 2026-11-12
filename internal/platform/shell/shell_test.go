package shell

import (
	"reflect"
	"testing"
)

// Parse gives each command its text, words, write targets and propagation, read from the syntax tree.
func TestParse(t *testing.T) {
	got, ast := Parse(`cd x && just test 2>&1 > out.txt | tail -3`, false)
	want := []Command{
		{Text: "cd x", Words: []string{"cd", "x"}, Literal: []bool{true, true}, Propagates: true},
		{Text: "just test 2>&1 > out.txt", Words: []string{"just", "test"}, Literal: []bool{true, true},
			Writes: []string{"out.txt"}, Redirects: true, Redirections: []string{"2>&1", "> out.txt"}},
		{Text: "tail -3", Words: []string{"tail", "-3"}, Literal: []bool{true, true}, Propagates: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse = %#v, want %#v", got, want)
	}
	if !ast {
		t.Error("the syntax tree read the line, not the heuristic")
	}
	if got[0].Nested() {
		t.Error("a command of the line is not nested")
	}
}

// The heuristic gives each command its text, words, write targets and propagation, and marks no word literal.
func TestFallback(t *testing.T) {
	got := fallback(`cd x && just test 2>&1 > out.txt | tail -3`)
	want := []Command{
		{Text: "cd x", Words: []string{"cd", "x"}, Literal: []bool{false, false}, Propagates: true},
		{Text: "just test 2>&1 > out.txt", Words: []string{"just", "test"}, Literal: []bool{false, false},
			Writes: []string{"out.txt"}, Redirects: true},
		{Text: "tail -3", Words: []string{"tail", "-3"}, Literal: []bool{false, false}, Propagates: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fallback = %#v, want %#v", got, want)
	}
}

// A line without commands gives none.
func TestParseEmpty(t *testing.T) {
	if got, _ := Parse("  ", false); len(got) != 0 {
		t.Errorf("Parse = %#v", got)
	}
}
