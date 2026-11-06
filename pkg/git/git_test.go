package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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
func gitRepo(t *testing.T) (dir string, git func(args ...string) string, write func(rel string)) {
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

// Branch, Ignored, ContentHash and Rel on one repository.
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
	if b := r.Branch(ctx); b != "main" {
		t.Fatalf("Branch = %q", b)
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
	if b := r.Branch(ctx); b != "" {
		t.Fatalf("Branch on a detached HEAD = %q", b)
	}
}

// Open fails outside a repository.
func TestOpenOutside(t *testing.T) {
	if _, err := Open(context.Background(), t.TempDir()); err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("Open outside = %v", err)
	}
}

// Branch and Ignored swallow the errors of git, so a context already done does not change their answer.
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
	if b := r.Branch(ctx); b != "main" {
		t.Fatalf("Branch with a cancelled context = %q", b)
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
