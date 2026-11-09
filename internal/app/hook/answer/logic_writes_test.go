package answer

import (
	"path/filepath"
	"slices"
	"testing"
)

// holdsAny sees a folder that holds what a pattern with a slash matches, and nothing for a pattern without one.
func TestHoldsAny(t *testing.T) {
	patterns := []string{"ai/rules/**", "docs/secret.md", ".env", "**/gen/*.go"}
	for dir, want := range map[string]bool{
		"ai/rules": true, "ai": true, "docs": true, "ai/other": false, "ai/rules/a.yaml": false,
		"docs/secret.md": false, "src": false, "gen": false,
	} {
		if _, got := holdsAny(patterns, dir, false); got != want {
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
		if _, got := wildcardHolds(patterns, word, false); got != want {
			t.Errorf("wildcardHolds(%q) = %v, want %v", word, got, want)
		}
	}
}

// Without case, a folder or a word in another case holds the same patterns.
func TestHoldsFold(t *testing.T) {
	if _, ok := holdsAny([]string{"AI/Rules/**"}, "ai", true); !ok {
		t.Error("holdsAny folds the case")
	}
	if _, ok := wildcardHolds([]string{"ai/rules/**"}, "AI/*", true); !ok {
		t.Error("wildcardHolds folds the case")
	}
}

// lineWrites finds what a line writes: the targets of redirections into the repository, and what a program that
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
		if _, _, got := lineWrites(patterns, c.command, "", root, nil, false, false); got != c.want {
			t.Errorf("writes(%q) = %v, want %v", c.command, got, c.want)
		}
	}
	if _, _, got := lineWrites([]string{"docs/secret.md"}, "chmod 644 *.md", "", root, nil, false, false); got {
		t.Error("*.md at the root does not reach docs/secret.md")
	}
}

// lineWrites resolves the words of the line from the cwd of the call and through the real paths it is given, so a
// root reached through a symlink still holds what it writes.
func TestWritesRealPaths(t *testing.T) {
	root, resolved := filepath.FromSlash("/link/repo"), filepath.FromSlash("/real/repo")
	cwd := filepath.Join(root, "ai")
	reals := map[string]string{root: resolved, filepath.Join(root, "ai", "rules", "x.yaml"): filepath.Join(resolved, "ai", "rules", "x.yaml")}
	p, _, ok := lineWrites([]string{"ai/rules/**"}, "echo x > rules/x.yaml", cwd, root, reals, false, false)
	if !ok || p != "ai/rules/x.yaml" {
		t.Fatalf("lineWrites = %q %v", p, ok)
	}
	if _, _, ok := lineWrites(nil, "rm -rf ai", cwd, root, reals, false, false); ok {
		t.Fatal("without patterns nothing is written")
	}
}

// writePaths lists, absolute, the redirections and the files of the programs that write by themselves.
func TestWritePaths(t *testing.T) {
	root := filepath.FromSlash("/repo")
	got := writePaths("echo x > a.txt && rm -rf ai && cat b.txt", "", root)
	want := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "ai")}
	if !slices.Equal(got, want) {
		t.Fatalf("writePaths = %q, want %q", got, want)
	}
}

// relative takes a relative path from the cwd, or the root without one, and tells a path outside the root.
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
		if got, inside := relative(root, c.cwd, c.p, nil); got != c.want || inside != c.inside {
			t.Errorf("relative(%q, %q) = %q %v, want %q %v", c.cwd, c.p, got, inside, c.want, c.inside)
		}
	}
	reals := map[string]string{root: filepath.FromSlash("/real")}
	if got, inside := relative(root, "", filepath.FromSlash("/real/x"), reals); got != "x" || !inside {
		t.Errorf("relative through the real root = %q %v", got, inside)
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
