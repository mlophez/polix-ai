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
	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/platform/build"
)

// UI writes the output meant for people: colors and symbols on a terminal, plain text elsewhere (a pipe, a file,
// CI, NO_COLOR), so scripts and tests read the same words. Output read by agents never uses it.
type UI struct {
	w io.Writer
}

// ExitCode is the error of a command that already told the user why it failed: it only carries the exit code,
// and Run prints nothing for it.
type ExitCode int

// Levels of a line of Status.
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
)

// NewUI returns the ui that writes to w, with the colors the terminal and the environment allow.
func NewUI(w io.Writer) *UI {
	return &UI{w: colorprofile.NewWriter(w, os.Environ())}
}

// Status prints one line of a report: the level, an area and the message.
func (u *UI) Status(level, area, msg string) {
	var badge string
	switch level {
	case LevelOK:
		badge = styleOK.Render("ok  ")
	case LevelWarn:
		badge = styleWarn.Render("warn")
	case LevelFail:
		badge = styleFail.Render("fail")
	default:
		badge = fmt.Sprintf("%-4s", level)
	}
	fmt.Fprintf(u.w, "%s  %s  %s\n", badge, styleMuted.Render(fmt.Sprintf("%-8s", area)), msg)
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

// Code styles a command or a path inside a line of the ui.
func Code(s string) string { return styleCode.Render(s) }

// OKText styles a word of a line as something that went well, in the color of OK.
func OKText(s string) string { return styleOK.Render(s) }

func (e ExitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// Exit turns the exit code of a command into its error: nil for 0, else an ExitCode.
func Exit(code int) error {
	if code == 0 {
		return nil
	}
	return ExitCode(code)
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
// and the version of build.Current, and returns the exit code: 0, the code of an ExitCode, or 1 for any other
// error, printed in one line. A nil stdin reads os.Stdin.
func Run(root *cobra.Command, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdin == nil {
		stdin = os.Stdin
	}
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := fang.Execute(context.Background(), root,
		fang.WithVersion(build.Current()),
		fang.WithoutManpage(),
		fang.WithErrorHandler(errorHandler(root.Name())),
	)
	if err == nil {
		return 0
	}
	if code, ok := errors.AsType[ExitCode](err); ok {
		return int(code)
	}
	return 1
}

// errorHandler prints the error of a command of the program name in one line, so scripts and agents can read
// it; an ExitCode was already explained by the command.
func errorHandler(name string) fang.ErrorHandler {
	return func(w io.Writer, _ fang.Styles, err error) {
		if _, ok := errors.AsType[ExitCode](err); ok {
			return
		}
		fmt.Fprintf(w, "%s %s\n", styleFail.Render(name+":"), err)
		if msg := err.Error(); strings.HasPrefix(msg, "unknown ") || strings.Contains(msg, "flag") || strings.Contains(msg, "arg(s)") {
			fmt.Fprintf(w, "%s\n", styleMuted.Render("Run "+name+" --help for usage."))
		}
	}
}
