package hook

import (
	"path/filepath"
	"slices"
	"testing"
)

// CommitDirs places each commit of a line made only of plain git commands in the folder git commits in, cwd after
// the -C options, and places nothing else: a line that moves the folder or the repository, expands something, or
// does not commit.
func TestCommitDirs(t *testing.T) {
	root, cwd := filepath.FromSlash("/repo"), filepath.FromSlash("/repo/sub")
	placed := []struct {
		command string
		want    []string
	}{
		{"git commit -m x", []string{cwd}},
		// Joined without cleaning: the real path resolves `..` after a symlink as git does.
		{"git -C ../../other commit -m x", []string{filepath.FromSlash("/repo/sub/../../other")}},
		{"git -C /tmp/a -C b commit", []string{filepath.FromSlash("/tmp/a/b")}},
		{"git -C /tmp/a commit -m 'fix (x) [y]? $z'", []string{filepath.FromSlash("/tmp/a")}},
		{`git -C /tmp/a commit -m "fix #12 (x)"`, []string{filepath.FromSlash("/tmp/a")}},
		{"git -C /tmp/a add -A && git -C /tmp/a commit -m x", []string{filepath.FromSlash("/tmp/a")}},
		{"git -C /tmp/a commit && git commit", []string{filepath.FromSlash("/tmp/a"), cwd}},
	}
	for _, c := range placed {
		if got, ok := CommitDirs(c.command, cwd, root, false); !ok || !slices.Equal(got, c.want) {
			t.Errorf("CommitDirs(%q) = %q, %v, want %q", c.command, got, ok, c.want)
		}
	}
	if got, ok := CommitDirs("git commit", "", root, false); !ok || !slices.Equal(got, []string{root}) {
		t.Errorf("without a cwd the commit is in the root: %q", got)
	}
	for _, command := range []string{
		"git status",
		"git commit-tree HEAD^{tree}",
		"git -C /tmp/a --git-dir /tmp/a/.git commit",
		"git --git-dir=/repo/.git -C /tmp/x commit",
		"git --work-tree /repo -C /tmp/x commit",
		"git -c core.worktree=/repo -C /tmp/x commit",
		"git -C ~/other commit",
		"git -C $HOME/other commit",
		"git -C /rep? commit",
		"git -C /re* commit",
		"git -C /[r]epo commit",
		"git -C /`pwd` commit",
		`git -C /tmp/x commit -m "$(git commit -q -m real)"`,
		"git -C /tmp/x commit -F <(git commit -q -m real)",
		"GIT_DIR=/repo/.git git -C /tmp/x commit",
		"export GIT_WORK_TREE=/repo; git -C /tmp/x commit",
		"cd internal/hook && git -C ../.. commit -m x",
		"{ cd internal/hook; git -C ../.. commit; }",
		"source ./x.sh && git -C /tmp/x commit",
		"ln -s /repo /tmp/l && git -C /tmp/l commit",
		"git worktree add /tmp/wt2 && git -C /tmp/wt2 commit",
		"git -C /tmp/a commit -m x && echo committed",
		`bash -c "git -C /tmp/a commit"`,
		"sudo git -C /tmp/a commit",
		// Comments, braces and backslashes, which bash reads otherwise than the words.
		"git -C /tmp/x commit -m a  # don't forget\ngit commit -m b",
		"git -C /tmp/x commit -m a && git -C /tmp/x {-C,/repo} commit -m b",
		`git -C /tmp/x commit -m a && git c\ommit -m b`,
		`git -C /tmp/x \-C /repo commit -m b`,
		`git -C /tmp/x commit -m "a\"b"`,
		"git -C /tmp/x commit -m 'unclosed",
		// Aliases, subcommands that run programs, settings and programs named git.
		"git -C /tmp/x commit -m a && git ci -m b",
		"git -C /tmp/x commit -m a && git rebase -x 'git commit' HEAD~1",
		"git -C /tmp/x commit -m a && git bisect run git commit -am b",
		"git -c user.name=a -C /tmp/a commit",
		"git --no-pager -C /tmp/a commit",
		"./bin/git -C /tmp/a commit",
		"/tmp/x/git -C /tmp/a commit",
	} {
		if got, ok := CommitDirs(command, cwd, root, false); ok {
			t.Errorf("CommitDirs(%q) placed %q", command, got)
		}
	}
	// On Windows a Git Bash path (/c/x) is not placed; a path with its drive is.
	for _, command := range []string{"git -C /c/Users/me/repo commit", `git -C 'C:\decoy' -C '\Users\me\repo' commit`,
		"git -C C:repo commit"} {
		if _, ok := CommitDirs(command, `C:\other`, `C:\repo`, true); ok {
			t.Errorf("CommitDirs(%q) placed on Windows", command)
		}
	}
	if _, ok := CommitDirs("git -C C:/other commit", `C:\repo`, `C:\repo`, true); !ok {
		t.Error("a path with its drive was not placed on Windows")
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

// writes finds what a line writes: the targets of redirections into the repository, and what a program that
// writes by itself names, its folders, wildcards and, for rm and the sources of mv, the root.
func TestWrites(t *testing.T) {
	root := t.TempDir()
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
		if _, _, got := Writes(patterns, c.command, "", root, nil, false, false); got != c.want {
			t.Errorf("writes(%q) = %v, want %v", c.command, got, c.want)
		}
	}
	if _, _, got := Writes([]string{"docs/secret.md"}, "chmod 644 *.md", "", root, nil, false, false); got {
		t.Error("*.md at the root does not reach docs/secret.md")
	}
}

// Writes resolves the words of the line from the cwd of the call and through the real paths it is given, so a root
// reached through a symlink still holds what it writes.
func TestWritesRealPaths(t *testing.T) {
	root, resolved := filepath.FromSlash("/link/repo"), filepath.FromSlash("/real/repo")
	cwd := filepath.Join(root, "ai")
	reals := map[string]string{root: resolved, filepath.Join(root, "ai", "rules", "x.yaml"): filepath.Join(resolved, "ai", "rules", "x.yaml")}
	p, _, ok := Writes([]string{"ai/rules/**"}, "echo x > rules/x.yaml", cwd, root, reals, false, false)
	if !ok || p != "ai/rules/x.yaml" {
		t.Fatalf("Writes = %q %v", p, ok)
	}
	if _, _, ok := Writes(nil, "rm -rf ai", cwd, root, reals, false, false); ok {
		t.Fatal("without patterns nothing is written")
	}
}

// WritePaths lists, absolute, the redirections and the files of the programs that write by themselves.
func TestWritePaths(t *testing.T) {
	root := filepath.FromSlash("/repo")
	got := WritePaths("echo x > a.txt && rm -rf ai && cat b.txt", "", root)
	want := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "ai")}
	if !slices.Equal(got, want) {
		t.Fatalf("WritePaths = %q, want %q", got, want)
	}
}

// Rel takes a relative path from the cwd, or the root without one, and tells a path outside the root.
func TestRel(t *testing.T) {
	root := filepath.FromSlash("/repo")
	for _, c := range []struct {
		cwd, p, want string
		inside       bool
	}{
		{"", "a/b.go", "a/b.go", true},
		{filepath.Join(root, "a"), "b.go", "a/b.go", true},
		{"", filepath.Join(root, "x"), "x", true},
		{"", filepath.FromSlash("/other/x"), "../other/x", false},
		{"", "", "", false},
	} {
		if got, inside := Rel(root, c.cwd, c.p, nil); got != c.want || inside != c.inside {
			t.Errorf("Rel(%q, %q) = %q %v, want %q %v", c.cwd, c.p, got, inside, c.want, c.inside)
		}
	}
	reals := map[string]string{root: filepath.FromSlash("/real")}
	if got, inside := Rel(root, "", filepath.FromSlash("/real/x"), reals); got != "x" || !inside {
		t.Errorf("Rel through the real root = %q %v", got, inside)
	}
}

// parentOnly sees the parents of the root, which hold it whole.
func TestParentOnly(t *testing.T) {
	for rel, want := range map[string]bool{"..": true, "../..": true, "../x": false, "": false, "x": false} {
		if got := parentOnly(rel); got != want {
			t.Errorf("parentOnly(%q) = %v", rel, got)
		}
	}
}
