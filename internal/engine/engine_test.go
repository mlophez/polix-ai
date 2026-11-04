package engine

import (
	"runtime"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/gitx"
)

// holdsAny sees a folder that holds what a pattern with a slash matches, and nothing for a pattern without one.
func TestHoldsAny(t *testing.T) {
	patterns := []string{"ai/rules/**", "docs/secret.md", ".env", "**/gen/*.go"}
	for dir, want := range map[string]bool{
		"ai/rules": true, "ai": true, "docs": true, "ai/other": false, "ai/rules/a.yaml": false,
		"docs/secret.md": false, "src": false, "gen": false,
	} {
		if _, got := holdsAny(patterns, dir); got != want {
			t.Errorf("holdsAny(%q) = %v, want %v", dir, got, want)
		}
	}
}

// wildcardHolds sees a word with wildcards that may write what a pattern matches, or a folder holding it.
func TestWildcardHolds(t *testing.T) {
	patterns := []string{"ai/rules/**", "docs/secret.md", ".env"}
	for word, want := range map[string]bool{
		"*": true, "ai/*": true, "ai/r*": true, "ai/rules/*": true, "ai/rules/sub/*.yaml": true,
		"docs/*.md": true, "docs/*.txt": false, "*.md": false, "ai/plans/*": false, "src/*.go": false, "*.log": false,
	} {
		if _, got := wildcardHolds(patterns, word); got != want {
			t.Errorf("wildcardHolds(%q) = %v, want %v", word, got, want)
		}
	}
}

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
	if !nullDevice("/dev/null") {
		t.Error("/dev/null discards everywhere")
	}
	if got, want := nullDevice("NUL"), runtime.GOOS == "windows"; got != want {
		t.Errorf("nullDevice(NUL) = %v on %s", got, runtime.GOOS)
	}
}

// writes finds what a line writes: the targets of redirections into the repository, and what a program that
// writes by itself names, its folders, wildcards and, for rm and the sources of mv, the root.
func TestWrites(t *testing.T) {
	root := t.TempDir()
	en := &Engine{Repo: &gitx.Repo{Root: root}}
	patterns := []string{".env", "AGENTS.md", "ai/rules/**", "docs/secret.md"}
	cases := []struct {
		command string
		want    bool
	}{
		{"echo x>AGENTS.md", true},
		{"echo x >&AGENTS.md", true},
		{"cat a.txt > ai/rules/x.yaml", true},
		{"rm -rf ai", true},
		{"rm -rf ai/*", true},
		{"rm -rf .", true},
		{"rm -rf " + root + "/..", true},
		{"mv . /tmp/x", true},
		{`rm -rf "a>b" ai`, true},
		{"sudo rm -rf ai/rules", true},
		{"git rm -r ai", true},
		{"sed -i s/a/b/ AGENTS.md", true},
		{"cp AGENTS.md notes.txt", true},
		{"ls ai 2>/dev/null", false},
		{"cat AGENTS.md > notes.txt", false},
		{"rg install docs", false},
		{"grep -rn rm ai", false},
		{"echo rm docs", false},
		{"mv /tmp/foo.txt .", false},
		{"mv docs/a.md ./", false},
		{"cp a.txt .", false},
		{"rm *.md", true}, // AGENTS.md is at the root
		{"rm *.txt", false},
		{`grep -n "x>y" AGENTS.md`, false},
		{"echo x >& 2", false},
		{`echo 'it'\''s' > AGENTS.md`, true},
		{`echo "a \"b" > AGENTS.md`, true},
		{`echo \"> AGENTS.md`, true},
		{"sed -Ei s/a/b/ AGENTS.md", true},
		{"perl -pi -e s/a/b/ AGENTS.md", true},
		{"perl -i.bak -pe s/a/b/ AGENTS.md", true},
		{"perl -Mlib=x script.pl AGENTS.md", false},
		{"perl -Ilib script.pl AGENTS.md", false},
		{"sed -n /x/p AGENTS.md", false},
		{"sed -e s/i/j/ AGENTS.md", false},
		{"perl script.pl -i AGENTS.md", false},
		{"perl -e s/a/b/ -i AGENTS.md", true},
		{"sudo \\\n  rm -rf ai/rules", true},
		{"LANG=C \\\n  rm -rf ai", true},
		{"echo x > \\\n  AGENTS.md", true},
	}
	for _, c := range cases {
		if _, _, got := en.writes(patterns, Event{Command: c.command}); got != c.want {
			t.Errorf("writes(%q) = %v, want %v", c.command, got, c.want)
		}
	}
	if _, _, got := en.writes([]string{"docs/secret.md"}, Event{Command: "chmod 644 *.md"}); got {
		t.Error("*.md at the root does not reach docs/secret.md")
	}
}
