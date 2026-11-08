package text

import "testing"

func TestWords(t *testing.T) {
	if got := Words("guardrails for coding agents"); got != 4 {
		t.Fatalf("Words = %d, want 4", got)
	}
}
