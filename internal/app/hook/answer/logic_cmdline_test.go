package answer

import (
	"slices"
	"testing"
)

// parseCommand keeps the words of a command apart from the targets of its redirections, respecting quotes.
func TestParseCommand(t *testing.T) {
	cases := []struct {
		command       string
		words, writes []string
	}{
		{"echo x >AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
		{"echo x>AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
		{"cat a.txt>>AGENTS.md", []string{"cat", "a.txt"}, []string{"AGENTS.md"}},
		{"echo x 2>> logs/out.txt", []string{"echo", "x"}, []string{"logs/out.txt"}},
		{"echo x >|AGENTS.md 2>&1", []string{"echo", "x"}, []string{"AGENTS.md"}},
		{"echo x >&AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
		{"echo x >& AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
		{"echo x >& 2 2>&-", []string{"echo", "x"}, nil},
		{"go test &> out.txt", []string{"go", "test"}, []string{"out.txt"}},
		{"ls ai 2>/dev/null", []string{"ls", "ai"}, []string{"/dev/null"}},
		{"sort < in.txt <<< x", []string{"sort"}, nil},
		{`grep -n "x>y" AGENTS.md`, []string{"grep", "-n", "x>y", "AGENTS.md"}, nil},
		{`rg 'Vec<T>::new' AGENTS.md`, []string{"rg", "Vec<T>::new", "AGENTS.md"}, nil},
		{`rm -rf "a>b" ai`, []string{"rm", "-rf", "a>b", "ai"}, nil},
		{`echo "2">x`, []string{"echo", "2"}, []string{"x"}},
		{`tee -a "ai/rules/x.yaml"`, []string{"tee", "-a", "ai/rules/x.yaml"}, nil},
		{`echo 'it'\''s' > AGENTS.md`, []string{"echo", "it's"}, []string{"AGENTS.md"}},
		{`echo "a \"b" > AGENTS.md`, []string{"echo", `a "b`}, []string{"AGENTS.md"}},
		{`echo \"> AGENTS.md`, []string{"echo", `"`}, []string{"AGENTS.md"}},
		{`echo a\>b`, []string{"echo", "a>b"}, nil},
		{"sudo \\\n  rm -rf ai", []string{"sudo", "rm", "-rf", "ai"}, nil},
		{"echo x > \\\n  AGENTS.md", []string{"echo", "x"}, []string{"AGENTS.md"}},
		{"rm -rf \\\nai", []string{"rm", "-rf", "ai"}, nil},
		{`type C:\Users\x\a.txt > out.txt`, []string{"type", `C:\Users\x\a.txt`}, []string{"out.txt"}},
	}
	for _, c := range cases {
		got := parseCommand(c.command)
		if !slices.Equal(got.words, c.words) || !slices.Equal(got.writes, c.writes) {
			t.Errorf("parseCommand(%q) = %q %q, want %q %q", c.command, got.words, got.writes, c.words, c.writes)
		}
	}
}

// program finds the program a command runs past assignments and wrappers, and git rm and git mv.
func TestProgram(t *testing.T) {
	for line, want := range map[string]string{
		"rm -rf ai": "rm", "LANG=C sudo -n /bin/rm x": "rm", "xargs rm -f": "rm", "git -C x rm -r ai": "rm",
		"git mv a b": "mv", "git status": "git", "rg install docs": "rg", "echo rm docs": "echo", "rm.exe x": "rm",
	} {
		if got, _ := parseCommand(line).program(); got != want {
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
	if segs := matchingCommands([]string{"just test*"}, "cd x && just test | tail -3"); len(segs) != 1 || propagates(segs) {
		t.Fatalf("piped: %+v", segs)
	}
	if segs := matchingCommands([]string{"just test*"}, "cd x && just test -k a"); len(segs) != 1 || !propagates(segs) {
		t.Fatalf("chained: %+v", segs)
	}
	if len(matchingCommands([]string{"git push*"}, `echo "git push"`)) != 0 {
		t.Fatal("the text of a command inside quotes")
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
		"polix config check 2>&1": true, "polix doctor 1>&2 | tail -3": false, "polix doctor 2>&1; polix version": true} {
		if got := onlyPolix(command); got != want {
			t.Errorf("onlyPolix(%q) = %v", command, got)
		}
	}
}
