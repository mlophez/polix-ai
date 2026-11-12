package git

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// TestMain runs the tests without the variables git exports to its hooks about the repository being committed, such as
// GIT_INDEX_FILE (.git/index, relative) in pre-commit: the githooks of polix.yaml run go test, and the repositories of
// these tests are their own, so git would read an index that is not theirs, or none in a linked worktree.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
	os.Exit(m.Run())
}

// Paths that are not plain ASCII come back as they are, not quoted and escaped by git.
func TestNonASCIIPaths(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	write("diseño/año.md")
	git("add", "--all")
	git("commit", "--quiet", "-m", "init")
	write("diseño/año.md.new")
	if err := os.WriteFile(filepath.Join(dir, "diseño", "año.md"), []byte("cambio"), 0o644); err != nil {
		t.Fatal(err)
	}

	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := r.Changed(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"diseño/año.md", "diseño/año.md.new"}; !slices.Equal(changed, want) {
		t.Errorf("Changed = %q, want %q", changed, want)
	}
	files, err := r.Files(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"diseño/año.md", "diseño/año.md.new"}; !slices.Equal(files, want) {
		t.Errorf("Files = %q, want %q", files, want)
	}
}

// gitRepo creates an empty repository in a temporary folder and returns it with a git runner and a writer.
func gitRepo(t testing.TB) (dir string, git func(args ...string) string, write func(rel string)) {
	t.Helper()
	dir = t.TempDir()
	git = func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write = func(rel string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "--quiet", "--initial-branch=main")
	return dir, git, write
}

// Only a folder of another repository is foreign: not a folder of this one, a linked worktree of it, a folder outside
// any repository nor a missing one.
func TestForeign(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "sub/a.txt")
	wt := filepath.Join(t.TempDir(), "wt")
	git("worktree", "add", "--quiet", "-b", "side", wt)
	other, _, _ := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	for folder, want := range map[string]bool{filepath.Join(dir, "sub"): false, wt: false, other: true,
		t.TempDir(): false, filepath.Join(other, "missing"): false} {
		if got := r.Foreign(ctx, folder); got != want {
			t.Errorf("Foreign(%s) = %v, want %v", folder, got, want)
		}
	}
	// A `..` after a link into this repository: Unix resolves it through the link, into this repository, where git
	// -C would commit; Windows drops it before following the link, so there the folder is the other repository.
	if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(other, "link")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	sep := string(filepath.Separator)
	through := other + sep + "link" + sep + ".."
	if got, want := r.Foreign(ctx, through), runtime.GOOS == "windows"; got != want {
		t.Errorf("Foreign(%s) = %v, want %v", through, got, want)
	}
	// A repository whose objects and refs link into ours commits into ours: not foreign.
	borrowed, _, _ := gitRepo(t)
	for _, name := range []string{"objects", "refs"} {
		p := filepath.Join(borrowed, ".git", name)
		if err := os.RemoveAll(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(dir, ".git", name), p); err != nil {
			t.Fatal(err)
		}
	}
	if r.Foreign(ctx, borrowed) {
		t.Errorf("Foreign(%s) = true for a repository whose objects and refs are ours", borrowed)
	}
}

func commit(git func(args ...string) string, write func(string), rel string) {
	write(rel)
	git("add", "--all")
	git("commit", "--quiet", "-m", rel)
}

// The closest candidate wins: a feature forked from dev, itself forked from main, compares with dev.
func TestResolveBaseClosest(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	git("checkout", "--quiet", "-b", "dev")
	commit(git, write, "b.txt")
	devTip := git("rev-parse", "HEAD")
	git("checkout", "--quiet", "-b", "feature")
	commit(git, write, "c.txt")

	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ResolveBase(ctx, []string{"main", "dev"})
	if err != nil || got != devTip {
		t.Fatalf("ResolveBase = %q, %v; want the tip of dev %q", got, err, devTip)
	}
	files, err := r.Changed(ctx, got)
	if err != nil || !slices.Equal(files, []string{"c.txt"}) {
		t.Fatalf("Changed(dev) = %q, %v", files, err)
	}
}

// A clone of one branch only has the base on origin.
func TestResolveBaseOrigin(t *testing.T) {
	ctx := context.Background()
	src, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	mainTip := git("rev-parse", "HEAD")
	dst := filepath.Join(t.TempDir(), "clone")
	git("clone", "--quiet", "--single-branch", src, dst)
	cl, err := Open(ctx, dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.git(ctx, "checkout", "--quiet", "-b", "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.git(ctx, "branch", "--quiet", "-D", "main"); err != nil {
		t.Fatal(err)
	}
	got, err := cl.ResolveBase(ctx, []string{"main"})
	if err != nil || got != mainTip {
		t.Fatalf("ResolveBase = %q, %v; want origin/main %q", got, err, mainTip)
	}
}

// Without any candidate, ResolveBase fails naming them.
func TestResolveBaseNone(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.ResolveBase(ctx, []string{"develop", "release"})
	if err == nil || !strings.Contains(err.Error(), "develop, release") {
		t.Fatalf("ResolveBase error = %v", err)
	}
}

// Changed without a ref lists only what is not committed yet.
func TestChangedSinceHead(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	git("checkout", "--quiet", "-b", "feature")
	commit(git, write, "b.txt")
	write("c.txt")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := r.Changed(ctx, "")
	if err != nil || !slices.Equal(files, []string{"a.txt", "c.txt"}) {
		t.Fatalf("Changed = %q, %v", files, err)
	}
}

// Head, Ignored, ContentHash and Rel on one repository.
func TestRepoFacts(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	write(".gitignore")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	head := git("rev-parse", "HEAD")
	if b, c := r.Head(ctx); b != "main" || c != head {
		t.Fatalf("Head = %q, %q, want main, %q", b, c, head)
	}
	if !r.Ignored(ctx, "build/x") || r.Ignored(ctx, "a.txt") {
		t.Fatal("Ignored does not follow .gitignore")
	}
	before, err := r.ContentHash(ctx, filepath.Join(r.GitDir, "polix"))
	if err != nil || before == "" {
		t.Fatalf("ContentHash = %q, %v", before, err)
	}
	write("b.txt")
	after, err := r.ContentHash(ctx, filepath.Join(r.GitDir, "polix"))
	if err != nil || after == before {
		t.Fatalf("ContentHash after an edit = %q, %v", after, err)
	}
	if rel, ok := r.Rel(filepath.Join(dir, "docs", "new.md"), ""); !ok || rel != "docs/new.md" {
		t.Fatalf("Rel = %q, %v", rel, ok)
	}
	if rel, ok := r.Rel("a.txt", filepath.Join(dir, "docs")); !ok || rel != "docs/a.txt" {
		t.Fatalf("Rel relative to cwd = %q, %v", rel, ok)
	}
	if _, ok := r.Rel(filepath.Join(dir, "..", "out.txt"), ""); ok {
		t.Fatal("a path outside the repository is not ok")
	}
	if _, ok := r.Rel("", ""); ok {
		t.Fatal("an empty path is not ok")
	}
	git("checkout", "--quiet", "--detach")
	if b, c := r.Head(ctx); b != "" || c != head {
		t.Fatalf("Head on a detached HEAD = %q, %q, want \"\", %q", b, c, head)
	}
}

// Refs is empty without a stash nor an operation stopped halfway, names the commit of refs/stash after a git stash,
// and adds the merge a conflict stopped, with its commit.
func TestRefs(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Refs(ctx); got != "" {
		t.Fatalf("Refs before any stash = %q", got)
	}
	change := func(content string) {
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	change("stashed")
	git("stash", "push", "--quiet")
	stash := "stash " + git("rev-parse", "refs/stash")
	if got := r.Refs(ctx); got != stash {
		t.Fatalf("Refs = %q, want %q", got, stash)
	}

	main := git("rev-parse", "--abbrev-ref", "HEAD")
	git("checkout", "--quiet", "-b", "other")
	change("other")
	git("commit", "--quiet", "-am", "other")
	other := git("rev-parse", "HEAD")
	git("checkout", "--quiet", main)
	change("main")
	git("commit", "--quiet", "-am", "main")
	cmd := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@example.com", "merge", "--quiet", "other")
	cmd.Dir = dir
	if err := cmd.Run(); err == nil {
		t.Fatal("the merge did not stop on its conflict")
	}
	if got, want := r.Refs(ctx), stash+", MERGE_HEAD "+other; got != want {
		t.Fatalf("Refs during a merge = %q, want %q", got, want)
	}
}

// Each operation that leaves the working tree changed without moving HEAD shows in Refs: a merge --squash, which
// makes no commit, a rebase stopped on its conflict (rebase-merge) and a git am stopped on its conflict
// (rebase-apply); aborting it clears it.
func TestRefsOperations(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name, want string
		run        []string
		abort      []string
	}{
		{"squash", "SQUASH_MSG ", []string{"merge", "--squash", "--quiet", "other"}, []string{"reset", "--quiet", "--hard"}},
		{"rebase", "rebase-merge", []string{"rebase", "other"}, []string{"rebase", "--abort"}},
		{"am", "rebase-apply", []string{"am", "patch"}, []string{"am", "--abort"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir, git, write := gitRepo(t)
			commit(git, write, "a.txt")
			change := func(rel, content string) {
				if err := os.WriteFile(filepath.Join(dir, rel), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			git("checkout", "--quiet", "-b", "other")
			change("a.txt", "other")
			git("commit", "--quiet", "-am", "other")
			patch := git("format-patch", "-1", "--stdout", "HEAD")
			git("checkout", "--quiet", "main")
			if c.name != "squash" {
				change("a.txt", "main")
				git("commit", "--quiet", "-am", "main")
				change("patch", patch+"\n")
			}
			r, err := Open(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			head := git("rev-parse", "HEAD")
			cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.com"}, c.run...)...)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull,
				"GIT_EDITOR=true")
			out, err := cmd.CombinedOutput()
			if c.name != "squash" && err == nil {
				t.Fatalf("git %v did not stop on its conflict:\n%s", c.run, out)
			}
			if got := git("rev-parse", "HEAD"); c.name != "rebase" && got != head {
				t.Fatalf("git %v moved HEAD from %s to %s", c.run, head, got)
			}
			got := r.Refs(ctx)
			if !strings.HasPrefix(got, c.want) {
				t.Fatalf("Refs during git %v = %q, want it to start with %q\n%s", c.run, got, c.want, out)
			}
			if again := r.Refs(ctx); again != got {
				t.Fatalf("Refs during git %v changed with nothing done: %q, then %q", c.run, got, again)
			}
			if c.name == "squash" && !regexp.MustCompile(`^SQUASH_MSG [0-9a-f]{64}$`).MatchString(got) {
				t.Fatalf("Refs during a merge --squash = %q, want SQUASH_MSG and the sha256 of its message", got)
			}
			git(c.abort...)
			if got := r.Refs(ctx); got != "" {
				t.Fatalf("Refs after git %v = %q", c.abort, got)
			}
		})
	}
}

// On a file system that folds case, a path a command renamed only in case is put back under its own name, not
// removed with the name the command gave it.
func TestRestoreCaseRename(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	upper, lower := filepath.Join(dir, "CLAUDE.md"), filepath.Join(dir, "claude.md")
	if err := os.WriteFile(upper, []byte("rules"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lower); err != nil {
		t.Skip("the file system tells case apart")
	}
	scratch := filepath.Join(r.GitDir, "polix")
	before, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(upper, lower); err != nil {
		t.Fatal(err)
	}
	after, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	paths, _, err := r.TreeDiff(ctx, before, after)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Restore(ctx, scratch, before, paths); err != nil {
		t.Fatalf("Restore %q: %v", paths, err)
	}
	if got, err := os.ReadFile(upper); err != nil || string(got) != "rules" {
		t.Fatalf("CLAUDE.md = %q, %v", got, err)
	}
}

// Head knows neither the branch nor the commit on a repository without commits.
func TestHeadWithoutCommits(t *testing.T) {
	ctx := context.Background()
	dir, _, _ := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, c := r.Head(ctx); b != "" || c != "" {
		t.Fatalf("Head without commits = %q, %q, want both empty", b, c)
	}
}

// TreeDiff lists the paths that differ between two content hashes, none for equal trees, and fails for a tree git
// does not have.
func TestTreeDiff(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "src/a.go")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	before, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	write("README.md")
	if err := os.WriteFile(filepath.Join(dir, "src", "a.go"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := r.ContentHash(ctx, scratch)
	if err != nil {
		t.Fatal(err)
	}
	paths, removed, err := r.TreeDiff(ctx, before, after)
	if err != nil || !slices.Equal(paths, []string{"README.md", "src/a.go"}) || len(removed) != 0 {
		t.Fatalf("TreeDiff = %q, %q, %v", paths, removed, err)
	}
	if paths, removed, err := r.TreeDiff(ctx, after, before); err != nil ||
		!slices.Equal(paths, []string{"README.md", "src/a.go"}) || !slices.Equal(removed, []string{"README.md"}) {
		t.Fatalf("TreeDiff back = %q, %q, %v", paths, removed, err)
	}
	if paths, _, err := r.TreeDiff(ctx, after, after); err != nil || len(paths) != 0 {
		t.Fatalf("TreeDiff of equal trees = %q, %v", paths, err)
	}
	missing := strings.Repeat("1", len(before))
	if _, _, err := r.TreeDiff(ctx, missing, after); err == nil {
		t.Fatal("a missing tree is an error")
	}
	if _, _, err := r.TreeDiff(ctx, "", after); err == nil {
		t.Fatal("an empty tree is an error")
	}
}

// A repository without an index yet (git init, no git add) has a content hash, the same for the same files, and the
// real index is never created; only the index of polix stays in the scratch folder.
func TestContentHashWithoutIndex(t *testing.T) {
	ctx := context.Background()
	dir, _, write := gitRepo(t)
	write("a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	first, err := r.ContentHash(ctx, scratch)
	if err != nil || first == "" {
		t.Fatalf("ContentHash without an index = %q, %v", first, err)
	}
	again, err := r.ContentHash(ctx, scratch)
	if err != nil || again != first {
		t.Fatalf("ContentHash again = %q, %v, want %q", again, err, first)
	}
	if _, err := os.Stat(filepath.Join(r.GitDir, "index")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("the real index is touched: %v", err)
	}
	if left, err := filepath.Glob(filepath.Join(scratch, "index-*")); err != nil || len(left) != 0 {
		t.Fatalf("the temporary index is left behind: %v %v", left, err)
	}
}

// BenchmarkContentHash measures the content hash of a repository of 3000 committed files and 200 untracked ones,
// with one file edited before each hash, as a hook sees it after an edit. The files are dated an hour back, as in a
// real repository: files written in the same second as an index are racily clean, and git reads them again on
// every call.
func BenchmarkContentHash(b *testing.B) {
	ctx := context.Background()
	dir, git, write := gitRepo(b)
	var files []string
	for i := range 3000 {
		files = append(files, filepath.ToSlash(filepath.Join("src", strconv.Itoa(i%30), strconv.Itoa(i)+".go")))
	}
	for i := range 200 {
		files = append(files, filepath.ToSlash(filepath.Join("new", strconv.Itoa(i)+".txt")))
	}
	past := time.Now().Add(-time.Hour)
	for _, f := range files {
		write(f)
		if err := os.Chtimes(filepath.Join(dir, filepath.FromSlash(f)), past, past); err != nil {
			b.Fatal(err)
		}
	}
	git("add", "--all", "src")
	git("commit", "--quiet", "-m", "files")
	r, err := Open(ctx, dir)
	if err != nil {
		b.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	edited := filepath.Join(dir, "src", "0", "0.go")
	if _, err := r.ContentHash(ctx, scratch); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := range b.N {
		if err := os.WriteFile(edited, []byte(strconv.Itoa(i)), 0o644); err != nil {
			b.Fatal(err)
		}
		if _, err := r.ContentHash(ctx, scratch); err != nil {
			b.Fatal(err)
		}
	}
}

// freshHash is the content hash computed from a new copy of the real index, outside the scratch folder: what
// ContentHash must return.
func freshHash(t *testing.T, r *Repo) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index")
	if err := r.copyIndex(path); err != nil {
		t.Fatal(err)
	}
	hash, err := stageTree(context.Background(), r.Root, path)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

// The copy of the index keeps the time of the real one, so git still compares the content of the files changed in
// the same tick as the real index (racily clean).
func TestCopyIndexKeepsTime(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Stat(filepath.Join(r.GitDir, "index"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "index")
	if err := r.copyIndex(path); err != nil {
		t.Fatal(err)
	}
	cp, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cp.ModTime().Equal(src.ModTime()) || cp.Size() != src.Size() {
		t.Fatalf("copy %v %d bytes, real index %v %d bytes", cp.ModTime(), cp.Size(), src.ModTime(), src.Size())
	}
}

// ContentHash gives the hash of a fresh copy of the real index through edits, new and deleted files, commits, and a
// file git stops tracking and ignores; and it never touches the real index.
func TestContentHashFollowsTheFiles(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	realIndex := func() string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(r.GitDir, "index"))
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(filepath.Join(r.GitDir, "index"))
		if err != nil {
			t.Fatal(err)
		}
		return info.ModTime().String() + string(data)
	}
	check := func(step string) string {
		t.Helper()
		before := realIndex()
		got, err := r.ContentHash(ctx, scratch)
		if err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if realIndex() != before {
			t.Fatalf("%s: the real index changed", step)
		}
		if want := freshHash(t, r); got != want {
			t.Fatalf("%s: ContentHash = %s, want %s", step, got, want)
		}
		return got
	}
	first := check("first call")
	if again := check("no change"); again != first {
		t.Fatalf("the hash changed without an edit: %s, %s", first, again)
	}
	write("b.txt")
	check("untracked file")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	check("edit")
	if err := os.Remove(filepath.Join(dir, "b.txt")); err != nil {
		t.Fatal(err)
	}
	check("deleted untracked file")
	commit(git, write, "c.txt")
	check("commit")
	// A tracked build output, then ignored and no longer tracked: it leaves the tree.
	commit(git, write, "out/x.bin")
	check("tracked output")
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("out/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("rm", "--quiet", "--cached", "-r", "out")
	check("output ignored and untracked")
}

// Calls at the same time, while the real index keeps changing under them, each return the hash of the files and
// leave no temporary index behind.
func TestContentHashConcurrent(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	write("b.txt")
	write("c.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(r.GitDir, "polix")
	want := freshHash(t, r)
	stop := make(chan struct{})
	toggled := make(chan struct{})
	go func() {
		defer close(toggled)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			// Staging and unstaging b.txt changes the real index, never the files: the hash stays the same.
			args := []string{"add", "b.txt"}
			if i%2 == 1 {
				args = []string{"reset", "--quiet", "b.txt"}
			}
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			_ = cmd.Run()
		}
	}()
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			for range 5 {
				got, err := r.ContentHash(ctx, scratch)
				if err == nil && got != want {
					err = errors.New("hash " + got + ", want " + want)
				}
				if err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}()
	}
	for range 8 {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
	close(stop)
	<-toggled
	if left, _ := filepath.Glob(filepath.Join(scratch, "index-*")); len(left) != 0 {
		t.Fatalf("temporary indexes are left behind: %v", left)
	}
}

// Only a sharing violation or a pending delete on Windows counts as a file held by another process, so the read of the
// index retries just then.
func TestLockedByOther(t *testing.T) {
	sharing := &fs.PathError{Op: "open", Path: "index", Err: syscall.Errno(32)}
	denied := &fs.PathError{Op: "open", Path: "index", Err: syscall.Errno(5)}
	missing := &fs.PathError{Op: "open", Path: "index", Err: fs.ErrNotExist}
	for _, c := range []struct {
		goos string
		err  error
		want bool
	}{
		{"windows", sharing, true},
		{"windows", denied, true},
		{"windows", missing, false},
		{"linux", sharing, false},
		{"darwin", denied, false},
	} {
		if got := lockedByOther(c.goos, c.err); got != c.want {
			t.Errorf("lockedByOther(%s, %v) = %v, want %v", c.goos, c.err, got, c.want)
		}
	}
}

// Open fails outside a repository.
func TestOpenOutside(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("Open outside = %v", err)
	}
}

// Head and Ignored swallow the errors of git, so a context already done does not change their answer.
func TestSwallowingCallsIgnoreCancel(t *testing.T) {
	dir, git, write := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("build/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit(git, write, "a.txt")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if b, c := r.Head(ctx); b != "main" || c == "" {
		t.Fatalf("Head with a cancelled context = %q, %q", b, c)
	}
	if !r.Ignored(ctx, "build/x") {
		t.Fatal("Ignored with a cancelled context lost the answer")
	}
}

// writeBytes writes data to the repository relative path rel under dir.
func writeBytes(t *testing.T, dir, rel string, data string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Stats counts the lines of the tracked changes, and reads the untracked files whole.
func TestStats(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	writeBytes(t, dir, ".gitignore", "build/\n")
	writeBytes(t, dir, "a.txt", "1\n2\n3\n")
	writeBytes(t, dir, "gone.txt", "x\ny\n")
	git("add", "--all")
	git("commit", "--quiet", "-m", "init")
	writeBytes(t, dir, "a.txt", "1\nnew\n3\nmore\n")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	writeBytes(t, dir, "dir con espacio/año.go", "one\ntwo")
	writeBytes(t, dir, "bin.dat", "a\x00b\n")
	writeBytes(t, dir, "build/out.txt", "ignored\n")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Stats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []Stat{
		{Path: "a.txt", Added: 2, Deleted: 1},
		{Path: "bin.dat", Binary: true},
		{Path: "dir con espacio/año.go", Added: 2},
		{Path: "gone.txt", Deleted: 2},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Stats = %+v, want %+v", got, want)
	}
}

// Without a commit, every tracked and untracked file counts whole.
func TestStatsWithoutCommits(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	writeBytes(t, dir, "a.txt", "1\n2\n")
	git("add", "--all")
	writeBytes(t, dir, "b.txt", "")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Stats(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if want := []Stat{{Path: "a.txt", Added: 2}, {Path: "b.txt"}}; !slices.Equal(got, want) {
		t.Fatalf("Stats = %+v, want %+v", got, want)
	}
}

// CommitStats measures a commit against its first parent, or a root commit against the empty tree.
func TestCommitStats(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	writeBytes(t, dir, "a.txt", "1\n2\n")
	git("add", "--all")
	git("commit", "--quiet", "-m", "root")
	root := git("rev-parse", "HEAD")
	writeBytes(t, dir, "a.txt", "1\nchanged\n")
	writeBytes(t, dir, "bin.dat", "\x00\x01")
	git("add", "--all")
	git("commit", "--quiet", "-m", "second")
	git("checkout", "--quiet", "-b", "side", root)
	writeBytes(t, dir, "side.txt", "s\n")
	git("add", "--all")
	git("commit", "--quiet", "-m", "side")
	git("checkout", "--quiet", "main")
	git("merge", "--quiet", "--no-ff", "--no-edit", "side")
	writeBytes(t, dir, "a.txt", "uncommitted\n")
	writeBytes(t, dir, "untracked.txt", "u\n")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]Stat{
		root:     {{Path: "a.txt", Added: 2}},
		"HEAD~1": {{Path: "a.txt", Added: 1, Deleted: 1}, {Path: "bin.dat", Binary: true}},
		"HEAD":   {{Path: "side.txt", Added: 1}},
	}
	for rev, want := range cases {
		got, err := r.CommitStats(ctx, rev)
		if err != nil {
			t.Fatalf("CommitStats(%s): %v", rev, err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("CommitStats(%s) = %+v, want %+v", rev, got, want)
		}
	}
	if _, err := r.CommitStats(ctx, "nope"); err == nil || !strings.Contains(err.Error(), `"nope" is not a commit`) {
		t.Fatalf("CommitStats of a missing revision = %v", err)
	}
	if s, err := r.Summary(ctx, "HEAD~1"); err != nil || !strings.HasSuffix(s, " second") {
		t.Fatalf("Summary = %q, %v", s, err)
	}
	if _, err := r.Summary(ctx, "--all"); err == nil {
		t.Fatal("Summary of an option is an error")
	}
}

// The staged files are the index against HEAD, a deleted one included: a file modified and not staged, or untracked,
// is left out. Without commits every staged file is.
func TestStaged(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	write("first.txt")
	git("add", "first.txt")
	write("untracked.txt")
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if files, err := r.Staged(ctx); err != nil || !slices.Equal(files, []string{"first.txt"}) {
		t.Fatalf("Staged without commits = %q, %v", files, err)
	}
	git("commit", "--quiet", "-m", "first")
	commit(git, write, "gone.txt")
	write("first.txt.new")
	if err := os.WriteFile(filepath.Join(dir, "first.txt"), []byte("not staged"), 0o644); err != nil {
		t.Fatal(err)
	}
	write("diseño/año.md")
	git("add", "first.txt.new", "diseño/año.md")
	git("rm", "--quiet", "gone.txt")
	if files, err := r.Staged(ctx); err != nil ||
		!slices.Equal(files, []string{"diseño/año.md", "first.txt.new", "gone.txt"}) {
		t.Fatalf("Staged = %q, %v", files, err)
	}
}

// The comment string is # by default and with auto, else core.commentChar, and core.commentString wins when both are
// set.
func TestCommentChar(t *testing.T) {
	ctx := context.Background()
	dir, git, _ := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if c := r.CommentChar(ctx); c != "#" {
		t.Fatalf("default: %q", c)
	}
	git("config", "core.commentChar", "auto")
	if c := r.CommentChar(ctx); c != "#" {
		t.Fatalf("auto: %q", c)
	}
	git("config", "core.commentChar", ";")
	if c := r.CommentChar(ctx); c != ";" {
		t.Fatalf("commentChar: %q", c)
	}
	git("config", "core.commentString", "//")
	if c := r.CommentChar(ctx); c != "//" {
		t.Fatalf("commentString: %q", c)
	}
}

// Branch is the current branch, also on a branch without commits, and "" on a detached HEAD.
func TestBranch(t *testing.T) {
	ctx := context.Background()
	dir, git, write := gitRepo(t)
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b := r.Branch(ctx); b != "main" {
		t.Fatalf("Branch without commits = %q", b)
	}
	commit(git, write, "a.txt")
	git("checkout", "--quiet", "-b", "PROJ-12-login")
	if b := r.Branch(ctx); b != "PROJ-12-login" {
		t.Fatalf("Branch = %q", b)
	}
	git("checkout", "--quiet", "--detach")
	if b := r.Branch(ctx); b != "" {
		t.Fatalf("Branch on a detached HEAD = %q", b)
	}
}
