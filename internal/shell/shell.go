// Package shell splits a command line into the commands it runs, so the patterns of the workflow match each
// command (`just test` in `cd x && just test | tail -3`) and the exit code of the line is only trusted for a
// command whose failure propagates to it.
//
// It is a heuristic, not a shell parser: quotes and backslashes are respected, but not every construct of bash.
package shell

import "strings"

// Segment is one command of a command line.
type Segment struct {
	Text string
	// Propagates is true when a failure of the command makes the whole line fail: it is the last command, or
	// it is followed by &&. After |, ;, || or &, the exit code of the line says nothing about it.
	Propagates bool
}

// Split returns the commands of a command line, in order, without empty ones.
func Split(line string) []Segment {
	var segs []Segment
	var cur strings.Builder
	flush := func(propagates bool) {
		if text := trimSubshell(cur.String()); text != "" {
			segs = append(segs, Segment{Text: text, Propagates: propagates})
		}
		cur.Reset()
	}
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' && i+1 < len(line) {
				cur.WriteByte(c)
				i++
				c = line[i]
			}
			cur.WriteByte(c)
			continue
		case c == '\'' || c == '"':
			quote = c
			cur.WriteByte(c)
			continue
		case c == '\\' && i+1 < len(line):
			cur.WriteByte(c)
			i++
			cur.WriteByte(line[i])
			continue
		}
		next := byte(0)
		if i+1 < len(line) {
			next = line[i+1]
		}
		switch {
		case c == '&' && next == '&':
			flush(true)
			i++
		case c == '|' && next == '|':
			flush(false)
			i++
		case c == '|' && next == '&': // |& pipes stderr too
			flush(false)
			i++
		case c == '>' && next == '&', c == '&' && next == '>', c == '>' && next == '|': // redirections: 2>&1, &>, >|
			cur.WriteByte(c)
			cur.WriteByte(next)
			i++
		case c == '|', c == ';', c == '&':
			flush(false)
		case c == '\n':
			flush(false)
		default:
			cur.WriteByte(c)
		}
	}
	flush(true)
	if n := len(segs); n > 0 {
		segs[n-1].Propagates = true
	}
	return segs
}

// trimSubshell removes the parentheses of a subshell around a command (`(cd x && go test)` gives `cd x` and
// `go test)`): every opening one at the start, and closing ones at the end only while they outnumber the opening
// ones, so `echo $(date)` keeps its own.
func trimSubshell(text string) string {
	text = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(text), "("))
	for strings.HasSuffix(text, ")") && strings.Count(text, ")") > strings.Count(text, "(") {
		text = strings.TrimSpace(strings.TrimSuffix(text, ")"))
	}
	return text
}
