package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	changed, err := r.Changed("main")
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
