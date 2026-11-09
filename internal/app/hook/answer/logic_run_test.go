package answer

import "testing"

// indent indents every line, and nothing for an empty text.
func TestIndent(t *testing.T) {
	if indent("a\nb\n") != "  a\n  b" || indent("") != "" {
		t.Fatal("indent")
	}
}
