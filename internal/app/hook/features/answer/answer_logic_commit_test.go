package answer

import (
	"path/filepath"
	"slices"
	"testing"
)

// commitDirs places each commit of a line made only of plain git commands in the folder git commits in, cwd after
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
		if got, ok := commitDirs(c.command, cwd, root, false); !ok || !slices.Equal(got, c.want) {
			t.Errorf("commitDirs(%q) = %q, %v, want %q", c.command, got, ok, c.want)
		}
	}
	if got, ok := commitDirs("git commit", "", root, false); !ok || !slices.Equal(got, []string{root}) {
		t.Errorf("without a cwd the commit is in the root: %q", got)
	}
	// AC10: the line is read as bash reads it, so a comment is no word of the command, and a comment, a backslash or a
	// quote escaped inside double quotes places each commit where bash runs it.
	for command, want := range map[string][]string{
		"git -C /tmp/x commit -m a  # don't forget\ngit commit -m b": {filepath.FromSlash("/tmp/x"), cwd},
		`git -C /tmp/x commit -m a && git c\ommit -m b`:              {filepath.FromSlash("/tmp/x"), cwd},
		`git -C /tmp/x \-C /repo commit -m b`:                        {filepath.FromSlash("/repo")},
		`git -C /tmp/x commit -m "a\"b"`:                             {filepath.FromSlash("/tmp/x")},
		"git -C ../other commit -m x # note":                         {filepath.FromSlash("/repo/sub/../other")},
		"git -C ../other add . ; git -C ../other commit -m 'a; b'":   {filepath.FromSlash("/repo/sub/../other")},
	} {
		if got, ok := commitDirs(command, cwd, root, false); !ok || !slices.Equal(got, want) {
			t.Errorf("commitDirs(%q) = %q, %v, want %q", command, got, ok, want)
		}
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
		// Braces, which bash expands into other words.
		"git -C /tmp/x commit -m a && git -C /tmp/x {-C,/repo} commit -m b",
		"git -C /tmp/x commit -m 'unclosed",
		// AC10 and a line the parser rejects, decided as before by the text.
		`git -C ../other commit -m "$(cat msg)"`, `git -C "$OTHER" commit`, "git -C ~/x commit",
		"git -C ../other commit > log", "git -C ../other commit 2>&1", `git -C ../other commit -m "a\"b`,
		// A clause fails closed, and the patterns of case are walked.
		"case x in $(rm -rf ai)) ;; esac; git -C ../other commit -m x",
		"if true; then git -C ../other commit -m x; fi", "f() { :; }; git -C ../other commit -m x",
		// Aliases, subcommands that run programs, settings and programs named git.
		"git -C /tmp/x commit -m a && git ci -m b",
		"git -C /tmp/x commit -m a && git rebase -x 'git commit' HEAD~1",
		"git -C /tmp/x commit -m a && git bisect run git commit -am b",
		"git -c user.name=a -C /tmp/a commit",
		"git --no-pager -C /tmp/a commit",
		"./bin/git -C /tmp/a commit",
		"/tmp/x/git -C /tmp/a commit",
	} {
		if got, ok := commitDirs(command, cwd, root, false); ok {
			t.Errorf("commitDirs(%q) placed %q", command, got)
		}
	}
	// On Windows a Git Bash path (/c/x) is not placed; a path with its drive is.
	for _, command := range []string{"git -C /c/Users/me/repo commit", `git -C 'C:\decoy' -C '\Users\me\repo' commit`,
		"git -C C:repo commit"} {
		if _, ok := commitDirs(command, `C:\other`, `C:\repo`, true); ok {
			t.Errorf("commitDirs(%q) placed on Windows", command)
		}
	}
	if _, ok := commitDirs("git -C C:/other commit", `C:\repo`, `C:\repo`, true); !ok {
		t.Error("a path with its drive was not placed on Windows")
	}
}
