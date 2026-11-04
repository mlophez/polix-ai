package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Paths that are not plain ASCII come back as they are, not quoted and escaped by git.
func TestNonASCIIPaths(t *testing.T) {
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

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := r.Changed("")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"diseño/año.md", "diseño/año.md.new"}; !slices.Equal(changed, want) {
		t.Errorf("Changed = %q, want %q", changed, want)
	}
	files, err := r.Files()
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
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	git("checkout", "--quiet", "-b", "dev")
	commit(git, write, "b.txt")
	devTip := git("rev-parse", "HEAD")
	git("checkout", "--quiet", "-b", "feature")
	commit(git, write, "c.txt")

	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.ResolveBase([]string{"main", "dev"})
	if err != nil || got != devTip {
		t.Fatalf("ResolveBase = %q, %v; want the tip of dev %q", got, err, devTip)
	}
	files, err := r.Changed(got)
	if err != nil || !slices.Equal(files, []string{"c.txt"}) {
		t.Fatalf("Changed(dev) = %q, %v", files, err)
	}
}

// A clone of one branch only has the base on origin.
func TestResolveBaseOrigin(t *testing.T) {
	src, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	mainTip := git("rev-parse", "HEAD")
	dst := filepath.Join(t.TempDir(), "clone")
	git("clone", "--quiet", "--single-branch", src, dst)
	cl, err := Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cl.git("checkout", "--quiet", "-b", "feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := cl.git("branch", "--quiet", "-D", "main"); err != nil {
		t.Fatal(err)
	}
	got, err := cl.ResolveBase([]string{"main"})
	if err != nil || got != mainTip {
		t.Fatalf("ResolveBase = %q, %v; want origin/main %q", got, err, mainTip)
	}
}

// Without any candidate, ResolveBase fails naming them.
func TestResolveBaseNone(t *testing.T) {
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = r.ResolveBase([]string{"develop", "release"})
	if err == nil || !strings.Contains(err.Error(), "develop, release") {
		t.Fatalf("ResolveBase error = %v", err)
	}
}

// Changed without a ref lists only what is not committed yet.
func TestChangedSinceHead(t *testing.T) {
	dir, git, write := gitRepo(t)
	commit(git, write, "a.txt")
	git("checkout", "--quiet", "-b", "feature")
	commit(git, write, "b.txt")
	write("c.txt")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	files, err := r.Changed("")
	if err != nil || !slices.Equal(files, []string{"a.txt", "c.txt"}) {
		t.Fatalf("Changed = %q, %v", files, err)
	}
}
