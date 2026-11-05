package rule

import (
	"fmt"
	"strings"

	"github.com/mlophez/polix-ai/pkg/text"
)

// Result is the outcome of running the check of one rule. It is domain, not of a use case: CheckRule and the hook
// that runs the rules of the changed files produce it alike. It is built with a literal followed by Valid.
type Result struct {
	Rule   Rule
	Output string // everything the check printed
	Err    error  // why the run failed: the script exited with an error, timed out or could not start; nil if it passed
}

const (
	// TailLines is how many lines of the output of a check Tail keeps, the last ones.
	TailLines = 15
	// MaxLineLen caps each line Tail keeps, in bytes.
	MaxLineLen = 500
)

// Valid reports why the result is not the result of a check: its rule must be valid and have a check.
func (r Result) Valid() error {
	if err := r.Rule.Valid(); err != nil {
		return err
	}
	if !r.Rule.HasCheck() {
		return fmt.Errorf("rule %s has no check to have a result", r.Rule.ID)
	}
	return nil
}

// Passed reports whether the check passed.
func (r Result) Passed() bool { return r.Err == nil }

// Tail is what is shown of the run: the last TailLines non-empty lines of the output followed by the error, each
// cut to MaxLineLen bytes.
func (r Result) Tail() string {
	text := r.Output
	if r.Err != nil {
		text += "\n" + r.Err.Error()
	}
	return tail(text, TailLines)
}

// Failed returns the results whose check failed, in their order.
func Failed(results []Result) []Result {
	var out []Result
	for _, r := range results {
		if !r.Passed() {
			out = append(out, r)
		}
	}
	return out
}

// tail keeps the last n non-empty lines of s, each cut to MaxLineLen bytes.
func tail(s string, n int) string {
	var lines []string
	for l := range strings.SplitSeq(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, text.Cut(l, MaxLineLen))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
