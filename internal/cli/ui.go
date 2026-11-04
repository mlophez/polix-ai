package cli

import (
	"fmt"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// ui writes the output meant for people: colors and symbols on a terminal, plain text elsewhere (a pipe, a file,
// CI, NO_COLOR), so scripts and tests read the same words. The hooks never use it: agents read their output.
type ui struct {
	w io.Writer
}

func newUI(w io.Writer) *ui {
	return &ui{w: colorprofile.NewWriter(w, os.Environ())}
}

var (
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleFail  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleCode  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
)

// status prints one line of a report: the level, an area and the message.
func (u *ui) status(level, area, msg string) {
	var badge string
	switch level {
	case levelOK:
		badge = styleOK.Render("ok  ")
	case levelWarn:
		badge = styleWarn.Render("warn")
	case levelFail:
		badge = styleFail.Render("fail")
	default:
		badge = fmt.Sprintf("%-4s", level)
	}
	fmt.Fprintf(u.w, "%s  %s  %s\n", badge, styleMuted.Render(fmt.Sprintf("%-8s", area)), msg)
}

func (u *ui) line(format string, args ...any) {
	fmt.Fprintf(u.w, format+"\n", args...)
}

func (u *ui) ok(format string, args ...any) {
	fmt.Fprintf(u.w, "%s %s\n", styleOK.Render("✓"), fmt.Sprintf(format, args...))
}

func (u *ui) warn(format string, args ...any) {
	fmt.Fprintf(u.w, "%s %s\n", styleWarn.Render("!"), fmt.Sprintf(format, args...))
}

func (u *ui) fail(format string, args ...any) {
	fmt.Fprintf(u.w, "%s %s\n", styleFail.Render("✗"), fmt.Sprintf(format, args...))
}

func code(s string) string { return styleCode.Render(s) }
