package shell

import "strings"

// segment is one command of a command line, as the splitter reads it.
type segment struct {
	text string
	// propagates is true when a failure of the command makes the whole line fail: it is the last command, or
	// it is followed by &&. After |, ;, || or &, the exit code of the line says nothing about it.
	propagates bool
}

// splitter walks a command line byte by byte: cur holds the command being read, quote the quote it is inside
// (0 outside quotes) and segs the commands already read.
type splitter struct {
	line  string
	quote byte
	cur   strings.Builder
	segs  []segment
}

// wordParser is parseWords reading a simple command one character at a time: the words and write targets read so
// far, the word being read, whether it holds a quoted or escaped character, the quote it is inside of and what the
// next word is.
type wordParser struct {
	rs     []rune
	words  []string
	writes []string
	cur    strings.Builder
	inWord bool
	quoted bool
	quote  rune
	// pending says what the next word is: a target that writes ("w"), the word of >& ("dup"), a word to skip
	// ("skip": the target of <, <<, <<< and <&) or a plain word ("").
	pending string
}

// split returns the commands of a command line, in order, without empty ones.
func split(line string) []segment {
	s := &splitter{line: line}
	for i := 0; i < len(line); i++ {
		i = s.step(i)
	}
	s.flush(true)
	if n := len(s.segs); n > 0 {
		s.segs[n-1].propagates = true
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
		s.segs = append(s.segs, segment{text: text, propagates: propagates})
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

// fallback reads a command line with the splitter and the word parser: the commands of split, each with its words
// and the targets of its redirections. It knows nothing of nested commands nor of expansions, so no word is literal.
func fallback(line string) []Command {
	segs := split(line)
	out := make([]Command, len(segs))
	for i, seg := range segs {
		words, writes := parseWords(seg.text)
		out[i] = Command{Text: seg.text, Words: words, Literal: make([]bool, len(words)), Writes: writes,
			Redirects: len(writes) > 0, Propagates: seg.propagates}
	}
	return out
}

// shellSpecial are the characters a backslash outside quotes makes literal.
const shellSpecial = " \t\n'\"\\$`<>|&;()*?[]{}#~!"

// parseWords splits a simple command into its words and the targets of its redirections. Outside quotes an
// operator of a redirection ends a word (`x>AGENTS.md` is x and a redirection to AGENTS.md), and a descriptor
// right before it (`2>>log`) belongs to it. `>&` followed by a digit or `-` duplicates or closes a descriptor and
// writes nothing; followed by anything else it writes that file, as in bash. An input (<), a here-document (<<)
// and a here-string (<<<) write nothing. | ; and & also end a word.
func parseWords(text string) (words, writes []string) {
	p := &wordParser{rs: []rune(text)}
	for i := 0; i < len(p.rs); i++ {
		i = p.read(i)
	}
	p.flush()
	return p.words, p.writes
}

// at returns the character at i, or 0 past the end.
func (p *wordParser) at(i int) rune {
	if i < len(p.rs) {
		return p.rs[i]
	}
	return 0
}

// read reads the character at i and returns the index of the last character it consumed.
func (p *wordParser) read(i int) int {
	ch := p.rs[i]
	switch {
	case p.quote != 0:
		return p.inQuote(i)
	case ch == '\\' && p.at(i+1) == '\n':
		return i + 1 // a line continuation: both characters go away, and the word goes on or ends as without them
	case ch == '\\' && p.at(i+1) != 0 && strings.ContainsRune(shellSpecial, p.at(i+1)):
		// A backslash outside quotes makes a character of the shell literal (`\'`, `\>`, `\ `); before any other
		// character it stays, so a path of Windows such as C:\Users\x keeps its separators.
		p.cur.WriteRune(p.rs[i+1])
		p.inWord, p.quoted = true, true
		return i + 1
	case ch == '>' || ch == '<' || ch == '&' && p.at(i+1) == '>':
		return p.redirect(i)
	}
	p.plain(ch)
	return i
}

// inQuote reads the character at i inside a quote and returns the index of the last character it consumed.
func (p *wordParser) inQuote(i int) int {
	ch := p.rs[i]
	switch {
	case ch == p.quote:
		p.quote = 0
	case ch == '\\' && p.quote == '"' && p.at(i+1) == '\n':
		return i + 1 // a line continuation: both characters go away
	case ch == '\\' && p.quote == '"' && strings.ContainsRune(`"\\$`+"`", p.at(i+1)):
		p.cur.WriteRune(p.rs[i+1])
		return i + 1
	default:
		p.cur.WriteRune(ch)
	}
	return i
}

// redirect reads the operator of a redirection at i, which ends the word before it unless that word is the
// descriptor of the redirection, and returns the index of its last character.
func (p *wordParser) redirect(i int) int {
	if p.inWord && !p.quoted && p.pending == "" && strings.Trim(p.cur.String(), "0123456789") == "" {
		p.cur.Reset() // the descriptor of the redirection
		p.inWord = false
	} else {
		p.flush()
	}
	switch p.rs[i] {
	case '&': // &> and &>>
		i++
		if p.at(i+1) == '>' {
			i++
		}
		p.pending = "w"
		return i
	case '<':
		return p.input(i)
	}
	return p.output(i)
}

// output reads an operator that starts with >: >& duplicates a descriptor or writes a file (dup), and >, >> and >|
// write.
func (p *wordParser) output(i int) int {
	p.pending = "w"
	switch p.at(i + 1) {
	case '&':
		p.pending = "dup"
		return i + 1
	case '>', '|':
		return i + 1
	}
	return i
}

// input reads an operator that starts with <: <> writes, and <, <<, <<< and <& write nothing.
func (p *wordParser) input(i int) int {
	p.pending = "skip"
	switch p.at(i + 1) {
	case '>':
		p.pending = "w"
		return i + 1
	case '<':
		i++
		if p.at(i+1) == '<' {
			i++
		}
		return i
	case '&':
		return i + 1
	}
	return i
}

// plain reads a character outside quotes that is no redirection: a quote opens, | ; and & end the word and what it
// was pending, a blank ends the word, and any other character goes on it.
func (p *wordParser) plain(ch rune) {
	switch ch {
	case '\'', '"':
		p.quote, p.inWord, p.quoted = ch, true, true
	case '|', ';', '&':
		p.flush()
		p.pending = ""
	case ' ', '\t', '\n':
		p.flush()
	default:
		p.cur.WriteRune(ch)
		p.inWord = true
	}
}

// flush ends the word being read, as a word of the command or the target of the pending redirection.
func (p *wordParser) flush() {
	if !p.inWord {
		return
	}
	w := p.cur.String()
	switch p.pending {
	case "w":
		p.writes = append(p.writes, w)
	case "dup":
		if strings.Trim(w, "0123456789") != "" && w != "-" {
			p.writes = append(p.writes, w)
		}
	case "skip":
	default:
		p.words = append(p.words, w)
	}
	p.pending = ""
	p.cur.Reset()
	p.inWord, p.quoted = false, false
}
