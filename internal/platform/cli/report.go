package cli

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// Report is a diagnostic report for people: an optional header, sections of rows and a count line.
type Report struct {
	Title    string // the header; empty prints none
	Sections []Section
}

// Section is a titled group of rows of a Report.
type Section struct {
	Title  string // printed in bold
	Detail string // printed muted after the title, such as the root of the repository; may be empty
	Rows   []Row
}

// Row is one line of a Section: its level, the area it is about, a message and what to do about it.
type Row struct {
	Level   string // LevelOK, LevelWarn or LevelFail
	Area    string // empty on every row leaves out the area column
	Message string
	Fix     string // printed on its own line below the message; may be empty
}

// wrapper holds the state of wrapLine: the lines done, the line being built and whether a cut happened.
type wrapper struct {
	room    int
	lines   []string
	line    string
	started bool // a line is being built
	wrapped bool // a cut happened, so a line that starts is a continuation
}

// minPlainLevel is the width of the level words of plain output: the longest of ok, warn and fail.
const minPlainLevel = 4

// Report prints r: the header and a blank line, each section with rows (its title, its rows and a blank line) and
// the count line. In plain output the level is a word; in rich output, a colored symbol.
func (u *UI) Report(r Report) {
	var sections []Section
	areaWidth, levelWidth := 0, 1
	if !u.rich {
		levelWidth = minPlainLevel
	}
	counts := map[string]int{}
	for _, s := range r.Sections {
		if len(s.Rows) == 0 {
			continue
		}
		sections = append(sections, s)
		for _, row := range s.Rows {
			areaWidth = max(areaWidth, utf8.RuneCountInString(row.Area))
			levelWidth = max(levelWidth, utf8.RuneCountInString(u.label(row.Level)))
			counts[row.Level]++
		}
	}
	var b strings.Builder
	if r.Title != "" {
		b.WriteString(u.style(styleBold, r.Title) + "\n\n")
	}
	for _, s := range sections {
		b.WriteString(u.style(styleBold, s.Title))
		if s.Detail != "" {
			b.WriteString("  " + u.style(styleMuted, s.Detail))
		}
		b.WriteString("\n")
		for _, row := range s.Rows {
			u.row(&b, row, levelWidth, areaWidth)
		}
		b.WriteString("\n")
	}
	u.count(&b, counts)
	fmt.Fprint(u.w, b.String())
}

// row writes one row: its line, and its fix on a line below, indented to the column of the message.
func (u *UI) row(b *strings.Builder, row Row, levelWidth, areaWidth int) {
	label := u.label(row.Level)
	b.WriteString("  " + u.style(levelStyle(row.Level), label) + pad(label, levelWidth) + "  ")
	column := 2 + levelWidth + 2
	if areaWidth > 0 {
		b.WriteString(u.style(styleMuted, row.Area) + pad(row.Area, areaWidth) + "  ")
		column += areaWidth + 2
	}
	b.WriteString(strings.Join(u.wrap(row.Message, column, "", nil), "\n"+strings.Repeat(" ", column)) + "\n")
	if row.Fix != "" {
		const arrow = "→ "
		lines := u.wrap(row.Fix, column+utf8.RuneCountInString(arrow), arrow, &styleCode)
		b.WriteString(strings.Repeat(" ", column) + strings.Join(lines, "\n"+strings.Repeat(" ", column)) + "\n")
	}
}

// wrap splits text into the lines that fit the room left after column, at word boundaries; a word longer than the
// room is broken at it. The first line starts with prefix, which the continuations replace with spaces. Each line
// takes st when it is not nil. Plain output, an unknown width and a room of zero or less do not wrap.
func (u *UI) wrap(text string, column int, prefix string, st *lipgloss.Style) []string {
	lines := []string{text}
	if room := u.width - column; u.rich && u.width > 0 && room > 0 {
		lines = wordWrap(text, room)
	}
	indent := strings.Repeat(" ", utf8.RuneCountInString(prefix))
	for i, line := range lines {
		if i == 0 {
			line = prefix + line
		} else {
			line = indent + line
		}
		if st != nil {
			line = u.style(*st, line)
		}
		lines[i] = line
	}
	return lines
}

// count writes the count line: how many rows are ok, warn and fail, each with the color of its level in rich output.
func (u *UI) count(b *strings.Builder, counts map[string]int) {
	sep := u.style(styleMuted, " · ")
	for n, level := range []string{LevelOK, LevelWarn, LevelFail} {
		if n > 0 {
			b.WriteString(sep)
		}
		b.WriteString(u.style(levelStyle(level), fmt.Sprintf("%d %s", counts[level], level)))
	}
	b.WriteString("\n")
}

// label is the level of a row: its word in plain output, its symbol in rich output for a known level.
func (u *UI) label(level string) string {
	if !u.rich {
		return level
	}
	switch level {
	case LevelOK:
		return "✓"
	case LevelWarn:
		return "!"
	case LevelFail:
		return "✗"
	}
	return level
}

// style renders s with st in rich output, and leaves it as it is in plain output.
func (u *UI) style(st lipgloss.Style, s string) string {
	if !u.rich {
		return s
	}
	return st.Render(s)
}

// levelStyle is the style of a level; unknown levels are not styled.
func levelStyle(level string) lipgloss.Style {
	switch level {
	case LevelOK:
		return styleOK
	case LevelWarn:
		return styleWarn
	case LevelFail:
		return styleFail
	}
	return lipgloss.NewStyle()
}

// wordWrap splits text into lines of at most room columns. It keeps the line breaks of text and leaves intact each
// line that fits; a line that does not fit is cut only at single spaces, so the other spacing stays as it is, and a
// word longer than room is broken at it.
func wordWrap(text string, room int) []string {
	var lines []string
	for src := range strings.SplitSeq(text, "\n") {
		if textWidth(src) <= room {
			lines = append(lines, src)
			continue
		}
		lines = append(lines, wrapLine(src, room)...)
	}
	return lines
}

// wrapLine cuts one line that does not fit at its single spaces; empty words keep the extra spaces inside a line, but
// are dropped at a cut, so a continuation never starts with whitespace nor is blank. The leading spaces of src stay
// attached to its first word, and a line made only of spaces is never added.
func wrapLine(src string, room int) []string {
	w := &wrapper{room: room}
	lead, trimmed := splitLead(src, room)
	for i, word := range strings.Split(trimmed, " ") {
		if i == 0 {
			word = lead + word
		}
		w.word(word)
	}
	if w.started {
		w.add(w.line)
	}
	if len(w.lines) == 0 {
		return []string{""}
	}
	return w.lines
}

// add appends l to the lines unless it is blank.
func (w *wrapper) add(l string) {
	if strings.TrimSpace(l) != "" {
		w.lines = append(w.lines, l)
	}
}

// cutLine ends the line being built, if any; the next line is a continuation.
func (w *wrapper) cutLine() {
	if w.started {
		w.add(w.line)
	}
	w.line, w.started, w.wrapped = "", false, true
}

// word adds one word, which may be empty, to the line being built, cutting where it does not fit.
func (w *wrapper) word(word string) {
	if w.wrapped && !w.started {
		word = startOfLine(word)
		if word == "" {
			return
		}
	}
	for textWidth(word) > w.room {
		if w.started {
			w.cutLine()
		} else {
			head, rest := cut(word, w.room)
			w.add(head)
			w.wrapped, word = true, rest
		}
		word = startOfLine(word)
	}
	switch {
	case !w.started:
		if word != "" || !w.wrapped {
			w.line, w.started = word, true
		}
	case word == "" && textWidth(w.line)+1 > w.room:
		w.cutLine()
	case textWidth(w.line)+1+textWidth(word) <= w.room:
		w.line += " " + word
	default:
		w.cutLine()
		if word = startOfLine(word); word != "" {
			w.line, w.started = word, true
		}
	}
}

// startOfLine drops the whitespace a word has at the start of a continuation line, so a line after a cut never starts
// with it.
func startOfLine(word string) string {
	return strings.TrimLeftFunc(word, unicode.IsSpace)
}

// splitLead splits the leading spaces of src from the rest; spaces as wide as the room are dropped.
func splitLead(src string, room int) (string, string) {
	trimmed := strings.TrimLeft(src, " ")
	lead := src[:len(src)-len(trimmed)]
	if textWidth(lead) >= room {
		lead = ""
	}
	return lead, trimmed
}

// cut splits word after the longest prefix of at most room columns, and at least one rune. It sizes each rune from
// the bytes of word, so invalid UTF-8 never goes past its end.
func cut(word string, room int) (string, string) {
	end := 0
	for end < len(word) {
		_, size := utf8.DecodeRuneInString(word[end:])
		if end > 0 && textWidth(word[:end+size]) > room {
			break
		}
		end += size
	}
	return word[:end], word[end:]
}

// pad returns the spaces that complete s to width columns.
func pad(s string, width int) string {
	return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}

// tabStop is the most columns a tab can take, the worst case of a terminal that moves the cursor to the next tab stop.
const tabStop = 8

// textWidth is the columns s takes on a terminal. lipgloss counts a tab as zero columns, so each tab counts the worst
// case, and a line that passes this measure never prints wider than its room.
func textWidth(s string) int {
	return lipgloss.Width(s) + tabStop*strings.Count(s, "\t")
}
