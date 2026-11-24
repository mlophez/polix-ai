// Package cli holds what the commands of a cobra program share: the ui for people (ok, warn and fail lines), the
// error that only carries an exit code, the flag --dir, the root command and Run, which executes it with the
// styles of fang and returns the exit code.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/fang"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"
)

// UI writes the output meant for people: colors and symbols on a terminal, plain text elsewhere (a pipe, a file,
// CI, NO_COLOR), so scripts and tests read the same words. Output read by agents never uses it.
type UI struct {
	w    io.Writer
	rich bool // whether the output has colors: symbols and styles instead of words
	// width is the columns of the terminal the output goes to; zero when unknown or not a terminal, and then nothing wraps
	width int
}

// ExitCodeError is the error of a command that already told the user why it failed: it only carries the exit code,
// and Run prints nothing for it.
type ExitCodeError int

// Tone is the color Paint gives a text; the zero Tone gives none.
type Tone int

// Levels of a row of a Report.
const (
	LevelOK   = "ok"
	LevelWarn = "warn"
	LevelFail = "fail"
)

var (
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	styleWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	styleFail  = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	styleMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleCode  = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	styleBold  = lipgloss.NewStyle().Bold(true)
)

// NewUI returns the ui that writes to w, with the colors the terminal and the environment allow: the output is rich
// when colorprofile detects colors for w, plain otherwise. When w is a terminal it reads its width, so rich output
// wraps long text; a width that cannot be read is zero, which never wraps.
func NewUI(w io.Writer) *UI {
	cw := colorprofile.NewWriter(w, os.Environ())
	return &UI{w: cw, rich: cw.Profile >= colorprofile.ANSI, width: terminalWidth(w)}
}

// terminalWidth is the width in columns of w when it is a terminal, and zero otherwise.
func terminalWidth(w io.Writer) int {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(f.Fd()) {
		return 0
	}
	width, _, err := term.GetSize(f.Fd())
	if err != nil || width < 0 {
		return 0
	}
	return width
}

// Line prints a plain line.
func (u *UI) Line(format string, args ...any) {
	fmt.Fprintf(u.w, format+"\n", args...)
}

// OK prints a line of something that went well.
func (u *UI) OK(format string, args ...any) {
	fmt.Fprintf(u.w, "%s %s\n", styleOK.Render("✓"), fmt.Sprintf(format, args...))
}

// Warn prints a line of something to look at.
func (u *UI) Warn(format string, args ...any) {
	fmt.Fprintf(u.w, "%s %s\n", styleWarn.Render("!"), fmt.Sprintf(format, args...))
}

// Fail prints a line of something that failed.
func (u *UI) Fail(format string, args ...any) {
	fmt.Fprintf(u.w, "%s %s\n", styleFail.Render("✗"), fmt.Sprintf(format, args...))
}

// Change prints a line of something a command changed: a green +, or a yellow - when it took something away, the
// action muted and padded, and what it changed.
func (u *UI) Change(removal bool, action, what string) {
	sign := styleOK.Render("+")
	if removal {
		sign = styleWarn.Render("-")
	}
	fmt.Fprintf(u.w, "%s %s %s\n", sign, styleMuted.Render(fmt.Sprintf("%-12s", action)), what)
}

// Tones of Paint.
const (
	ToneRed   Tone = iota + 1 // something refused or stopped
	ToneGreen                 // something that went well
	ToneMuted                 // something to see only when looked for
)

// Paint returns s in the color of tone when the output of the ui has colors, and s as it is otherwise or for a tone
// that has none.
func (u *UI) Paint(tone Tone, s string) string {
	if !u.rich {
		return s
	}
	switch tone {
	case ToneRed:
		return styleFail.Render(s)
	case ToneGreen:
		return styleOK.Render(s)
	case ToneMuted:
		return styleMuted.Render(s)
	}
	return s
}

// Code styles a command or a path inside a line of the ui.
func Code(s string) string { return styleCode.Render(s) }

// OKText styles a word of a line as something that went well, in the color of OK.
func OKText(s string) string { return styleOK.Render(s) }

func (e ExitCodeError) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// Exit turns the exit code of a command into its error: nil for 0, else an ExitCodeError.
func Exit(code int) error {
	if code == 0 {
		return nil
	}
	return ExitCodeError(code)
}

// DirFlag adds --dir (-C), the repository a command works on, with def as its default.
func DirFlag(c *cobra.Command, dir *string, def string) {
	c.Flags().StringVarP(dir, "dir", "C", def, "repository to work on")
	_ = c.MarkFlagDirname("dir")
}

// Root returns a root command that leaves the usage and the errors to Run.
func Root(use, short, long, example string) *cobra.Command {
	return &cobra.Command{
		Use:           use,
		Short:         short,
		Long:          long,
		Example:       example,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
}

// Run executes root with args (without the program name) over stdin, stdout and stderr, with the styles of fang
// and the version the caller sets in root.Version, and returns the exit code: 0, the code of an ExitCodeError, or 1
// for any other error, printed in one line. A nil stdin reads os.Stdin.
func Run(root *cobra.Command, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil {
		stdin = os.Stdin
	}
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := fang.Execute(context.Background(), root,
		fang.WithVersion(root.Version),
		fang.WithoutManpage(),
		fang.WithErrorHandler(errorHandler(root.Name())),
	)
	if err == nil {
		return 0
	}
	if code, ok := errors.AsType[ExitCodeError](err); ok {
		return int(code)
	}
	return 1
}

// errorHandler prints the error of a command of the program name in one line, so scripts and agents can read
// it; an ExitCodeError was already explained by the command.
func errorHandler(name string) fang.ErrorHandler {
	return func(w io.Writer, _ fang.Styles, err error) {
		if _, ok := errors.AsType[ExitCodeError](err); ok {
			return
		}
		fmt.Fprintf(w, "%s %s\n", styleFail.Render(name+":"), err)
		if msg := err.Error(); strings.HasPrefix(msg, "unknown ") || strings.Contains(msg, "flag") || strings.Contains(msg, "arg(s)") {
			fmt.Fprintf(w, "%s\n", styleMuted.Render("Run "+name+" --help for usage."))
		}
	}
}
