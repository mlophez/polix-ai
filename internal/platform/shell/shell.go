// Package shell splits a command line into the commands it runs, so a pattern can match each
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

// splitter walks a command line byte by byte: cur holds the command being read, quote the quote it is inside
// (0 outside quotes) and segs the commands already read.
type splitter struct {
	line  string
	quote byte
	cur   strings.Builder
	segs  []Segment
}

// Split returns the commands of a command line, in order, without empty ones.
func Split(line string) []Segment {
	s := &splitter{line: line}
	for i := 0; i < len(line); i++ {
		i = s.step(i)
	}
	s.flush(true)
	if n := len(s.segs); n > 0 {
		s.segs[n-1].Propagates = true
	}
	return s.segs
}

// step reads the byte at i, and the next one when both form one token, and returns the index of the last byte
// it read.
func (s *splitter) step(i int) int {
	c := s.line[i]
	switch {
	case s.quote != 0:
		return s.quoted(i)
	case c == '\'' || c == '"':
		s.quote = c
		s.cur.WriteByte(c)
		return i
	case c == '\\' && i+1 < len(s.line):
		return s.escaped(i)
	}
	return s.operator(i)
}

// quoted reads the byte at i inside quotes: the closing quote ends them, and a backslash inside double quotes
// keeps the byte after it, so an escaped quote does not close them.
func (s *splitter) quoted(i int) int {
	c := s.line[i]
	if c == s.quote {
		s.quote = 0
	} else if c == '\\' && s.quote == '"' && i+1 < len(s.line) {
		return s.escaped(i)
	}
	s.cur.WriteByte(c)
	return i
}

// escaped keeps the backslash at i and the byte after it in the command, whatever that byte is.
func (s *splitter) escaped(i int) int {
	s.cur.WriteByte(s.line[i])
	s.cur.WriteByte(s.line[i+1])
	return i + 1
}

// operator reads the byte at i outside quotes: an operator that separates commands ends the current one, a
// redirection that looks like one is kept, and any other byte belongs to the command.
func (s *splitter) operator(i int) int {
	c, next := s.line[i], byte(0)
	if i+1 < len(s.line) {
		next = s.line[i+1]
	}
	switch {
	case c == '&' && next == '&':
		s.flush(true)
		return i + 1
	case c == '|' && (next == '|' || next == '&'): // || and |&, which pipes stderr too
		s.flush(false)
		return i + 1
	case c == '>' && next == '&', c == '&' && next == '>', c == '>' && next == '|': // redirections: 2>&1, &>, >|
		s.cur.WriteByte(c)
		s.cur.WriteByte(next)
		return i + 1
	case c == '|', c == ';', c == '&', c == '\n':
		s.flush(false)
	default:
		s.cur.WriteByte(c)
	}
	return i
}

// flush ends the current command, which is kept unless it is empty once its subshell parentheses are trimmed.
func (s *splitter) flush(propagates bool) {
	if text := trimSubshell(s.cur.String()); text != "" {
		s.segs = append(s.segs, Segment{Text: text, Propagates: propagates})
	}
	s.cur.Reset()
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
