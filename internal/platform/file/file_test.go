package file

import (
	"os"
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
