package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mlophez/polix-ai/internal/platform/build"
)

// root returns a root command with one subcommand, run, that returns err and records the --dir it got.
func root(err error, dir *string) *cobra.Command {
	r := Root("polix", "short", "long", "example")
	run := &cobra.Command{
		Use:  "run",
		Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return err },
	}
	DirFlag(run, dir, ".")
	r.AddCommand(run)
	return r
}

func execute(err error, args ...string) (code int, dir, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Run(root(err, &dir), args, strings.NewReader(""), &out, &errOut)
	return code, dir, out.String(), errOut.String()
}

func TestRunSuccess(t *testing.T) {
	code, dir, _, stderr := execute(nil, "run", "-C", "repo")
	if code != 0 || dir != "repo" || stderr != "" {
		t.Fatalf("code %d, dir %q, stderr %q", code, dir, stderr)
	}
	if _, dir, _, _ := execute(nil, "run"); dir != "." {
		t.Fatalf("default dir %q", dir)
	}
}

// An ExitCodeError sets the exit code and prints nothing: the command already told why.
func TestRunExitCode(t *testing.T) {
	code, _, _, stderr := execute(Exit(2), "run")
	if code != 2 || stderr != "" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
	if Exit(0) != nil {
		t.Fatal("Exit(0) is no error")
	}
	if got := Exit(3).Error(); got != "exit status 3" {
		t.Fatalf("Exit(3) = %q", got)
	}
}

// Any other error exits with 1 and is printed in one line.
func TestRunError(t *testing.T) {
	code, _, _, stderr := execute(errors.New("boom"), "run")
	if code != 1 || stderr != "polix: boom\n" {
		t.Fatalf("code %d, stderr %q", code, stderr)
	}
}

// A wrong flag, argument or command points to the help.
func TestRunUsageErrors(t *testing.T) {
	for _, args := range [][]string{{"run", "--nope"}, {"run", "extra"}, {"nope"}} {
		code, _, _, stderr := execute(nil, args...)
		if code != 1 || !strings.HasPrefix(stderr, "polix: ") || !strings.HasSuffix(stderr, "Run polix --help for usage.\n") {
			t.Errorf("%v: code %d, stderr %q", args, code, stderr)
		}
	}
}

func TestRunVersion(t *testing.T) {
	_, _, stdout, _ := execute(nil, "--version")
	if !strings.Contains(stdout, build.Current()) {
		t.Fatalf("--version = %q", stdout)
	}
}

// Outside a terminal the ui writes plain text.
func TestUI(t *testing.T) {
	var b bytes.Buffer
	u := NewUI(&b)
	u.OK("done %d", 1)
	u.Warn("look")
	u.Fail("broken")
	u.Line("plain %s", Code("polix check"))
	u.Status(LevelOK, "git", "found")
	u.Status(LevelWarn, "hooks", "old")
	u.Status(LevelFail, "plugin", "missing")
	u.Status("info", "x", "y")
	u.Change(false, "created", "a.json")
	u.Change(true, "removed", OKText("b.json"))
	want := "✓ done 1\n! look\n✗ broken\nplain polix check\n" +
		"ok    git       found\nwarn  hooks     old\nfail  plugin    missing\ninfo  x         y\n" +
		"+ created      a.json\n- removed      b.json\n"
	if b.String() != want {
		t.Fatalf("ui wrote %q, want %q", b.String(), want)
	}
}

// The error names the program of the root command.
func TestRunErrorNamesProgram(t *testing.T) {
	r := Root("tool", "", "", "")
	r.AddCommand(&cobra.Command{Use: "run", RunE: func(*cobra.Command, []string) error { return nil }})
	var out, errOut bytes.Buffer
	code := Run(r, []string{"run", "--nope"}, strings.NewReader(""), &out, &errOut)
	if code != 1 || !strings.HasPrefix(errOut.String(), "tool: ") || !strings.Contains(errOut.String(), "Run tool --help") {
		t.Fatalf("code %d, stderr %q", code, errOut.String())
	}
}
