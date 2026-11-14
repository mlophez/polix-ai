package file

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestReadAndWrite(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b.json")
	if data, ok, err := Read(p); data != nil || ok || err != nil {
		t.Fatal("a missing file is empty", ok, err)
	}
	if err := Write(p, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if data, ok, err := Read(p); string(data) != "x" || !ok || err != nil {
		t.Fatal(string(data), ok, err)
	}
	if info, _ := os.Stat(p); runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Fatal(info.Mode())
	}
	if !Exists(p) || !IsDir(filepath.Dir(p)) || IsDir(p) {
		t.Fatal("exists, is dir")
	}
}

// A symlink stays, and its target keeps its mode.
func TestWriteKeepsLinkAndMode(t *testing.T) {
	target := filepath.Join(t.TempDir(), "t")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "l")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := Write(link, []byte("new")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(link)
	data, _ := os.ReadFile(target)
	if info.Mode()&os.ModeSymlink == 0 || string(data) != "new" {
		t.Fatal("the link stays and its target is written")
	}
	if info, _ := os.Stat(target); runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatal(info.Mode())
	}
}

func TestSync(t *testing.T) {
	dir := t.TempDir()
	c, u, r, err := Sync(dir, map[string][]byte{"a/x": []byte("1"), "b": []byte("2")})
	if err != nil || len(c) != 2 || u != nil || r != nil {
		t.Fatal(c, u, r, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gone", "y"), nil, 0o644); err == nil {
		t.Fatal("no folder yet")
	}
	_ = os.MkdirAll(filepath.Join(dir, "gone"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "gone", "y"), nil, 0o644)
	c, u, r, err = Sync(dir, map[string][]byte{"a/x": []byte("1"), "b": []byte("3")})
	if err != nil || c != nil || !slices.Equal(u, []string{filepath.Join(dir, "b")}) ||
		!slices.Equal(r, []string{filepath.Join(dir, "gone", "y")}) || IsDir(filepath.Join(dir, "gone")) {
		t.Fatal(c, u, r, err)
	}
}

func TestReadDir(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.yaml"), []byte("a"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0o644)
	files, err := ReadDir(dir, ".yaml")
	if err != nil || len(files) != 1 || string(files["a.yaml"]) != "a" {
		t.Fatal(files, err)
	}
	if files, err := ReadDir(filepath.Join(dir, "none"), ".yaml"); files != nil || err != nil {
		t.Fatal("a missing folder has none")
	}
}

func TestCacheFile(t *testing.T) {
	t.Setenv("X_CACHE", "/c")
	if p, err := CacheFile("X_CACHE", "app", "f.json"); err != nil || p != filepath.Join("/c", "f.json") {
		t.Fatal(p, err)
	}
	t.Setenv("X_CACHE", "")
	if p, err := CacheFile("X_CACHE", "app", "f.json"); err == nil && filepath.Base(filepath.Dir(p)) != "app" {
		t.Fatal(p)
	}
}

func TestWalk(t *testing.T) {
	base := t.TempDir()
	if err := Write(filepath.Join(base, "secret", "a.md"), []byte("secret")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "repo")
	for _, rel := range []string{"a/b/c.txt", "a/d.txt", "e/f.txt", ".git/HEAD", "top.txt"} {
		if err := Write(filepath.Join(root, filepath.FromSlash(rel)), []byte(rel)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(root, filepath.Join(root, "e", "loop")); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name    string
		folders []string
		want    []string
	}{
		{"whole root", []string{""}, []string{".git/HEAD", "a/b/c.txt", "a/d.txt", "e/f.txt", "e/loop", "top.txt"}},
		{"nested walked once", []string{"a/b", "a", "a"}, []string{"a/b/c.txt", "a/d.txt"}},
		{"missing adds nothing", []string{"none", "e"}, []string{"e/f.txt", "e/loop"}},
		{"empty folder", []string{"empty"}, nil},
		{"git folder", []string{".git"}, []string{".git/HEAD"}},
		{"root wins", []string{"a", ""}, []string{".git/HEAD", "a/b/c.txt", "a/d.txt", "e/f.txt", "e/loop", "top.txt"}},
		{"outside the root skipped", []string{"../secret", "a/../../secret", "/../secret", "e"}, []string{"e/f.txt", "e/loop"}},
		{"cleaned inside the root", []string{"a/b/../b", "./e/"}, []string{"a/b/c.txt", "e/f.txt", "e/loop"}},
		{"dot is the root", []string{"."}, []string{".git/HEAD", "a/b/c.txt", "a/d.txt", "e/f.txt", "e/loop", "top.txt"}},
		{"file in an intermediate segment", []string{"top.txt/sub", "a/d.txt/sub"}, nil},
	}
	for _, c := range cases {
		got := Walk(root, c.folders)
		if runtime.GOOS == "windows" {
			c.want = slices.DeleteFunc(c.want, func(p string) bool { return p == "e/loop" })
		}
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: Walk = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWalkDoesNotFollowLinkInFolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symbolic links need privileges here")
	}
	base := t.TempDir()
	if err := Write(filepath.Join(base, "outside", "sub", "secret.md"), []byte("secret")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "repo")
	if err := Write(filepath.Join(root, "a", "b.md"), []byte("b")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "outside"), filepath.Join(root, "a", "inner")); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		folders []string
		want    []string
	}{
		{"link as the folder", []string{"link"}, []string{"link"}},
		{"link in an intermediate segment", []string{"link/sub"}, []string{"link"}},
		{"nested link in an intermediate segment", []string{"a/inner/sub", "a/inner/sub/deeper"}, []string{"a/inner"}},
		{"link beside a real folder", []string{"a", "link/sub"}, []string{"a/b.md", "a/inner", "link"}},
		{"missing under a real folder", []string{"a/none/sub"}, nil},
	}
	for _, c := range cases {
		if got := Walk(root, c.folders); !slices.Equal(got, c.want) {
			t.Errorf("%s: Walk = %q, want %q", c.name, got, c.want)
		}
	}
}

// A junction is ModeIrregular, not ModeSymlink, since Go 1.23, and mklink /J needs no privileges.
func TestWalkDoesNotFollowJunctionInFolder(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junctions are only on Windows")
	}
	base := t.TempDir()
	if err := Write(filepath.Join(base, "outside", "sub", "secret.md"), []byte("secret")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "repo")
	if err := Write(filepath.Join(root, "a", "b.md"), []byte("b")); err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{filepath.Join(root, "ai"), filepath.Join(root, "a", "inner")} {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, filepath.Join(base, "outside")).CombinedOutput(); err != nil {
			t.Fatalf("mklink /J %s: %v: %s", link, err, out)
		}
	}

	cases := []struct {
		name    string
		folders []string
		want    []string
	}{
		{"junction as the folder", []string{"ai"}, []string{"ai"}},
		{"junction in an intermediate segment", []string{"ai/sub"}, []string{"ai"}},
		{"nested junction in an intermediate segment", []string{"a/inner/sub"}, []string{"a/inner"}},
		{"junction inside a walked folder", []string{"a"}, []string{"a/b.md", "a/inner"}},
		{"whole root", []string{""}, []string{"a/b.md", "a/inner", "ai"}},
	}
	for _, c := range cases {
		if got := Walk(root, c.folders); !slices.Equal(got, c.want) {
			t.Errorf("%s: Walk = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWalkMatchesFolderCaseExactly(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"docs/a.md", "docs/sub/b.md"} {
		if err := Write(filepath.Join(root, filepath.FromSlash(rel)), []byte(rel)); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		name    string
		folders []string
		want    []string
	}{
		{"exact case", []string{"docs"}, []string{"docs/a.md", "docs/sub/b.md"}},
		{"wrong case of the folder", []string{"Docs"}, nil},
		{"wrong case of an inner segment", []string{"docs/Sub"}, nil},
		{"wrong case of an outer segment", []string{"DOCS/sub"}, nil},
		{"wrong case beside the right one", []string{"Docs", "docs/sub"}, []string{"docs/sub/b.md"}},
	}
	for _, c := range cases {
		if got := Walk(root, c.folders); !slices.Equal(got, c.want) {
			t.Errorf("%s: Walk = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestWalkSkipsUnreadableFolder(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissions do not deny reading here")
	}
	root := t.TempDir()
	for _, rel := range []string{"a/locked/x.txt", "a/open.txt", "b/c.txt"} {
		if err := Write(filepath.Join(root, filepath.FromSlash(rel)), []byte(rel)); err != nil {
			t.Fatal(err)
		}
	}
	locked := filepath.Join(root, "a", "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })

	want := []string{"a/open.txt", "b/c.txt"}
	for _, folders := range [][]string{{""}, {"a", "b"}, {"a/locked", "a", "b"}} {
		if got := Walk(root, folders); !slices.Equal(got, want) {
			t.Errorf("Walk(%q) = %q, want %q", folders, got, want)
		}
	}
}
