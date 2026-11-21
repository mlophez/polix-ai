package cli

import (
	"fmt"
	"strings"
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
	b.WriteString(row.Message + "\n")
	if row.Fix != "" {
		b.WriteString(strings.Repeat(" ", column) + u.style(styleCode, "→ "+row.Fix) + "\n")
	}
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

// pad returns the spaces that complete s to width columns.
func pad(s string, width int) string {
	return strings.Repeat(" ", max(0, width-utf8.RuneCountInString(s)))
}
