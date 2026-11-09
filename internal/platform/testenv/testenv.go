// Package testenv is support for the tests of polix: files read and written in one call, a repository with one
// commit, and a machine of its own for a test (a home, a cache and a PATH with git and the fake CLIs of the
// agents, built from testdata/fakecli, which record their calls). Only test files import it.
package testenv

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	// environ is the environment of the test binary before any test changes it, in which the fake CLI is built.
	environ = os.Environ()
	// goTool is the go tool on the PATH of the test binary, and goToolErr the error of looking it up.
	goTool, goToolErr = exec.LookPath("go")
	// cacheDir is the cache folder of the user before any test changes HOME, where the fake CLI is built.
	cacheDir, cacheDirErr = os.UserCacheDir()

	// fakeOnce builds the fake CLI of the agents once per test binary, into fakeCLI or with the error errFake.
	fakeOnce sync.Once
	fakeCLI  string
	errFake  error
)

// Read returns the content of p, failing the test when it cannot be read.
func Read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Write creates p with data, and the folders that hold it.
func Write(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Machine gives a test a home, a cache and a PATH of its own: git, and the fake CLIs of the agents asked, which
// record their calls. It returns the folder of their state, and skips the test when git or the go tool is missing.
func Machine(t *testing.T, claude, codex bool) string {
	t.Helper()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not on the PATH")
	}
	if goToolErr != nil {
		t.Skip("the go tool is not on the PATH")
	}
	fakeOnce.Do(func() { fakeCLI, errFake = buildFakeCLI() })
	if errFake != nil {
		t.Fatal(errFake)
	}
	bin, st := t.TempDir(), t.TempDir()
	path := bin
	if err := os.Symlink(git, filepath.Join(bin, filepath.Base(git))); err != nil {
		path += string(os.PathListSeparator) + filepath.Dir(git) // Windows without symlinks: the folder of git
	}
	if claude {
		copyFile(t, fakeCLI, filepath.Join(bin, "claude"+exeSuffix()))
	}
	if codex {
		copyFile(t, fakeCLI, filepath.Join(bin, "codex"+exeSuffix()))
	}
	t.Setenv("PATH", path)
	t.Setenv("FAKE_STATE", st)
	Home(t)
	return st
}

// Home gives a test a home and a cache of its own.
func Home(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
}

// Calls returns the calls recorded by the fake CLIs in the folder of their state st.
func Calls(st string) []string {
	data, _ := os.ReadFile(filepath.Join(st, "calls"))
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

// GitRepo creates a repository with one commit on main.
func GitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "--quiet", "--initial-branch=main"},
		{"-c", "user.name=t", "-c", "user.email=t@x", "commit", "--quiet", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	return resolved
}

// buildFakeCLI builds testdata/fakecli, next to the source of this package, into a folder of the cache of the user
// named after its source and system, so the test binaries of several packages share one build and a change of
// the source builds it again. The folder is private to the user (0700): a shared folder such as the system temp
// would let another user plant the binary the tests put on the PATH.
func buildFakeCLI() (string, error) {
	if cacheDirErr != nil {
		return "", fmt.Errorf("testenv: no cache folder for the fake CLI: %w", cacheDirErr)
	}
	_, self, _, ok := runtime.Caller(0)
	if !ok || !filepath.IsAbs(self) {
		return "", errors.New("testenv: cannot find the source of testdata/fakecli (built with -trimpath?)")
	}
	src := filepath.Join(filepath.Dir(self), "testdata", "fakecli")
	code, err := os.ReadFile(filepath.Join(src, "main.go"))
	if err != nil {
		return "", fmt.Errorf("testenv: %w", err)
	}
	sum := sha256.Sum256(code)
	dir := filepath.Join(cacheDir, "polix-testenv",
		"fakecli-"+hex.EncodeToString(sum[:6])+"-"+runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("testenv: %w", err)
	}
	// Chmod fails on a folder of another user, and narrows one that already existed with wider permissions.
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("testenv: %w", err)
	}
	bin := filepath.Join(dir, "fakecli"+exeSuffix())
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}
	tmp, err := os.MkdirTemp(dir, "build-")
	if err != nil {
		return "", fmt.Errorf("testenv: %w", err)
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "fakecli"+exeSuffix())
	cmd := exec.Command(goTool, "build", "-o", out, ".")
	cmd.Dir = src
	cmd.Env = environ
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("testenv: go build fakecli: %w\n%s", err, output)
	}
	if err := os.Rename(out, bin); err != nil {
		if _, statErr := os.Stat(bin); statErr == nil {
			return bin, nil // another test binary built it first
		}
		return "", fmt.Errorf("testenv: %w", err)
	}
	return bin, nil
}

// copyFile copies the program src to dst.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// exeSuffix is the extension of programs on this system.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
