package answer

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/shell"
)

// program finds the program a command runs past assignments and wrappers, and git rm and git mv.
func TestProgram(t *testing.T) {
	for line, want := range map[string]string{
		"rm -rf ai": "rm", "LANG=C sudo -n /bin/rm x": "rm", "xargs rm -f": "rm", "git -C x rm -r ai": "rm",
		"git mv a b": "mv", "git status": "git", "rg install docs": "rg", "echo rm docs": "echo", "rm.exe x": "rm",
		"X=1 sudo git --no-pager -c k=v mv a b": "mv", "env A=1 git -C x rm a": "rm",
	} {
		cmds, _ := shell.Parse(line, false)
		if got, _ := program(cmds[0]); got != want {
			t.Errorf("program(%q) = %q, want %q", line, got, want)
		}
	}
}

// NUL is the null device only on Windows; elsewhere it is a file like any other.
func TestNullDevice(t *testing.T) {
	if !nullDevice("/dev/null", false) {
		t.Error("/dev/null discards everywhere")
	}
	if !nullDevice("NUL", true) || !nullDevice("nul", true) {
		t.Error("NUL discards on Windows")
	}
	if nullDevice("NUL", false) {
		t.Error("NUL is a file elsewhere")
	}
}

// A pattern matches each command of a line, and its exit code is trusted only when its failure propagates.
func TestMatchingCommands(t *testing.T) {
	if segs := matchingCommands([]string{"just test*"}, "cd x && just test | tail -3", false); len(segs) != 1 ||
		propagates(segs) {
		t.Fatalf("piped: %+v", segs)
	}
	if segs := matchingCommands([]string{"just test*"}, "cd x && just test -k a", false); len(segs) != 1 ||
		!propagates(segs) {
		t.Fatalf("chained: %+v", segs)
	}
	if len(matchingCommands([]string{"git push*"}, `echo "git push"`, false)) != 0 {
		t.Fatal("the text of a command inside quotes")
	}
	// A pattern reaches every line of a message, written in quotes or in a here-document of a substitution.
	heredoc := "git commit -m \"$(cat <<'EOF'\nFix x\n\nCo-Authored-By: Claude <a@b>\nEOF\n)\""
	if len(matchingCommands([]string{"git commit*Co-Authored-By: Claude*"}, heredoc, false)) != 1 {
		t.Fatal("a multiline message in a here-document")
	}
}

// Only the commands that check the workflow or update polix pass an invalid workflow.
func TestOnlyPolix(t *testing.T) {
	for command, want := range map[string]bool{"polix config check": true, "cd . && polix config check --dir .": true,
		"polix update --version 0.2.0": true, "polix version": true, "polix doctor": true, "ls": false,
		"polix config check && ls": false, "polix check": false, "polix config fmt --check": true, "polix config fmt": false, "polix config fmt --check=false": false,
		"polix config fmt --check --check=false": false, "": false,
		// A substitution or a redirection could run or write anything behind an allowed command.
		"polix config check $(polix config fmt)": false, "polix config check `polix config fmt`": false,
		"polix config check > polix.yaml": false, "polix doctor >> polix.yaml": false, "polix version < x": false,
		"polix config check 2>&1": true, "polix doctor 1>&2 | tail -3": false, "polix doctor 2>&1; polix version": true,
		// AC9: the line is read as bash reads it; a quoted > is text, and every nested command must fit too.
		`polix doctor "x>y"`: true, "cd x && polix doctor": true, "polix doctor $(rm -rf ai)": false,
		"polix config check > out.txt": false, "(cd x && polix doctor)": true, "polix doctor <<< x": false,
		"polix version | rm -rf ai": false, "if polix doctor; then rm -rf ai; fi": false,
		// A clause fails closed, even with allowed commands, and the patterns of case are walked.
		"case x in $(rm -rf ai)) polix doctor;; esac": false, "if polix doctor; then polix version; fi": false,
		// A line the parser rejects is decided as before by the text.
		`polix doctor "unclosed`: true, `polix doctor "x>y`: false, `polix doctor "$(rm -rf ai)`: false} {
		if got := onlyPolix(command, false); got != want {
			t.Errorf("onlyPolix(%q) = %v", command, got)
		}
	}
}
