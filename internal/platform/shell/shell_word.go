package shell

import (
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ansiEscapes are the escapes of one letter of a $'...' word.
var ansiEscapes = map[byte]string{'a': "\a", 'b': "\b", 'e': "\x1b", 'E': "\x1b", 'f': "\f", 'n': "\n", 'r': "\r",
	't': "\t", 'v': "\v", '\\': "\\", '\'': "'", '"': "\"", '?': "?"}

// assign returns the text of an assignment as a word, `NAME=value`, and whether it is literal; one with an index or
// an array keeps its text as written and is not.
func (r *reader) assign(a *syntax.Assign) (string, bool) {
	if a.Index != nil || a.Array != nil || a.Name == nil {
		if a.Name == nil && a.Value != nil && a.Index == nil && a.Array == nil {
			return r.word(a.Value) // a naked argument of a declaration: an option or a name
		}
		return r.source(a), false
	}
	op := "="
	if a.Append {
		op = "+="
	}
	if a.Naked {
		return a.Name.Value, true
	}
	if a.Value == nil {
		return a.Name.Value + op, true
	}
	text, literal := r.word(a.Value)
	return a.Name.Value + op + text, literal
}

// word returns the text of a word as the command receives it, quotes removed, and whether it is literal. A word with
// an expansion ($x, $(...), $((...)), a process substitution or an extended glob) keeps its text as written; one
// with a glob, a brace or a tilde outside quotes is decoded but not literal.
func (r *reader) word(w *syntax.Word) (string, bool) {
	var b strings.Builder
	literal := true
	for _, part := range w.Parts {
		switch p := part.(type) {
		case *syntax.Lit:
			text, plain := r.unquoted(p.Value)
			b.WriteString(text)
			literal = literal && plain
		case *syntax.SglQuoted:
			if p.Dollar {
				b.WriteString(ansiC(p.Value))
			} else {
				b.WriteString(p.Value)
			}
		case *syntax.DblQuoted:
			if !doubleQuoted(p, &b) {
				return r.source(w), false
			}
		default:
			return r.source(w), false
		}
	}
	return b.String(), literal
}

// unquoted decodes a literal outside quotes: a backslash makes the character after it literal, and goes away with
// it; on Windows it stays before a character that is not special to the shell, so C:\Users\x keeps its separators.
// plain is false when the literal holds a glob, a brace or a tilde that is not escaped.
func (r *reader) unquoted(v string) (text string, plain bool) {
	var b strings.Builder
	plain = true
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch {
		case c == '\\' && i+1 < len(v) && v[i+1] == '\n':
			i++
		case c == '\\' && i+1 < len(v):
			if r.windows && !strings.ContainsRune(shellSpecial, rune(v[i+1])) {
				b.WriteByte(c)
			}
			b.WriteByte(v[i+1])
			i++
		default:
			if strings.IndexByte("*?[{~", c) >= 0 {
				plain = false
			}
			b.WriteByte(c)
		}
	}
	return b.String(), plain
}

// doubleQuoted writes the text of a double quoted part to b, and returns false when it holds an expansion. Inside
// double quotes a backslash only escapes ", \, $, a backtick and a newline.
func doubleQuoted(q *syntax.DblQuoted, b *strings.Builder) bool {
	for _, part := range q.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			return false
		}
		v := lit.Value
		for i := 0; i < len(v); i++ {
			if v[i] == '\\' && i+1 < len(v) && strings.IndexByte("\"\\$`\n", v[i+1]) >= 0 {
				if v[i+1] != '\n' {
					b.WriteByte(v[i+1])
				}
				i++
				continue
			}
			b.WriteByte(v[i])
		}
	}
	return true
}

// ansiC decodes the text of a $'...' word as bash does: the escapes of one letter, \xHH, \uHHHH, \UHHHHHHHH and the
// octal \nnn; any other backslash stays.
func ansiC(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 == len(v) {
			b.WriteByte(v[i])
			continue
		}
		e := v[i+1]
		if s, ok := ansiEscapes[e]; ok {
			b.WriteString(s)
			i++
			continue
		}
		n := ansiNumber(v[i+1:], &b)
		if n == 0 {
			b.WriteByte('\\')
			continue
		}
		i += n
	}
	return b.String()
}

// ansiNumber decodes a numeric escape of $'...' at the start of v, after its backslash: x with up to 2 hex digits,
// u with up to 4, U with up to 8, or up to 3 octal digits. It writes the character to b and returns how many bytes it
// read, 0 when v starts no numeric escape.
func ansiNumber(v string, b *strings.Builder) int {
	base, digits, skip, valid := 16, 0, 1, "0123456789abcdefABCDEF"
	switch v[0] {
	case 'x':
		digits = 2
	case 'u':
		digits = 4
	case 'U':
		digits = 8
	default:
		base, digits, skip, valid = 8, 3, 0, "01234567"
	}
	end := skip
	for end < len(v) && end-skip < digits && strings.IndexByte(valid, v[end]) >= 0 {
		end++
	}
	if end == skip {
		return 0
	}
	n, err := strconv.ParseUint(v[skip:end], base, 32)
	if err != nil {
		return 0
	}
	if v[0] == 'x' || base == 8 {
		b.WriteByte(byte(n))
	} else {
		b.WriteRune(rune(n))
	}
	return end
}

// literals returns n words marked literal.
func literals(n int) []bool {
	out := make([]bool, n)
	for i := range out {
		out[i] = true
	}
	return out
}
