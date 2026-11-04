package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/update"
)

// fakeRelease serves a releases page whose latest release, v0.2.0, holds files with their checksums.txt, and
// counts the requests.
type fakeRelease struct {
	srv      *httptest.Server
	requests atomic.Int32
}

// asset is the name of the binary of this platform in a release.
var asset = update.AssetName(runtime.GOOS, runtime.GOARCH)

// newFakeRelease serves bin as the binary of this platform.
func newFakeRelease(t *testing.T, bin []byte) *fakeRelease {
	return newFakeReleaseFiles(t, map[string][]byte{asset: bin}, nil)
}

// newFakeReleaseFiles serves files; sums overrides the checksum of some of them, to serve a tampered release.
func newFakeReleaseFiles(t *testing.T, files map[string][]byte, sums map[string][]byte) *fakeRelease {
	t.Helper()
	var listing strings.Builder
	for name, data := range files {
		if fake, ok := sums[name]; ok {
			data = fake
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&listing, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	f := &fakeRelease{}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v0.2.0", http.StatusFound)
	})
	for _, prefix := range []string{"/download/v0.2.0/", "/latest/download/"} {
		mux.HandleFunc(prefix+"checksums.txt", func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(listing.String()))
		})
		for name, data := range files {
			mux.HandleFunc(prefix+name, func(w http.ResponseWriter, r *http.Request) { w.Write(data) })
		}
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	t.Setenv("POLIX_RELEASES_URL", f.srv.URL)
	return f
}

// fakeBinary makes the update work on a copy instead of the test binary, with version as the running one.
func fakeBinary(t *testing.T, version string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe, oldVersion := executable, Version
	executable, Version = func() (string, error) { return exe, nil }, version
	t.Cleanup(func() { executable, Version = oldExe, oldVersion })
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
	t.Setenv("POLIX_NO_UPDATE", "")
	return exe
}

func runCmd(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(args, nil, &out, &errb)
	return code, out.String(), errb.String()
}

func content(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestUpdate(t *testing.T) {
	newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.1.0")

	code, stdout, stderr := runCmd("update", "--check")
	if code != 0 || !strings.Contains(stdout, "can be updated to 0.2.0") || content(t, exe) != "old" {
		t.Fatalf("--check only reports: %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runCmd("update")
	if code != 0 || !strings.Contains(stdout, "updated from 0.1.0 to 0.2.0") || content(t, exe) != "new" {
		t.Fatalf("update: %d %q %q", code, stdout, stderr)
	}

	Version = "0.2.0"
	code, stdout, _ = runCmd("update")
	if code != 0 || !strings.Contains(stdout, "up to date") {
		t.Fatalf("up to date: %d %q", code, stdout)
	}
}

// --version installs a given release, older ones included, and a development build is only replaced that way.
func TestUpdateVersion(t *testing.T) {
	newFakeRelease(t, []byte("pinned"))
	exe := fakeBinary(t, "dev")
	code, _, stderr := runCmd("update")
	if code != 1 || !strings.Contains(stderr, "development build") || content(t, exe) != "old" {
		t.Fatalf("a dev build is not replaced by the latest: %d %q", code, stderr)
	}
	code, _, stderr = runCmd("update", "--version", "v0.2.0")
	if code != 0 || content(t, exe) != "pinned" {
		t.Fatalf("--version: %d %q", code, stderr)
	}
}

// The hook mode checks at most once a day, never fails and says nothing when there is nothing to say.
func TestUpdateHook(t *testing.T) {
	rel := newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.1.0")

	code, stdout, _ := runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stdout, "actualizado de 0.1.0 a 0.2.0") || content(t, exe) != "new" {
		t.Fatalf("first hook updates: %d %q", code, stdout)
	}
	n := rel.requests.Load()
	Version = "0.1.0"
	code, stdout, _ = runCmd("update", "--hook")
	if code != 0 || stdout != "" || rel.requests.Load() != n {
		t.Fatalf("a second hook the same day does nothing: %d %q, %d requests", code, stdout, rel.requests.Load()-n)
	}

	if err := update.SaveCache(update.Cache{CheckedAt: time.Now().Add(-48 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLIX_RELEASES_URL", "http://127.0.0.1:1") // nothing listens
	code, stdout, _ = runCmd("update", "--hook")
	if code != 0 || stdout != "" {
		t.Fatalf("without network the hook stays quiet: %d %q", code, stdout)
	}

	t.Setenv("POLIX_NO_UPDATE", "1")
	if code, _, stderr := runCmd("update"); code != 1 || !strings.Contains(stderr, "POLIX_NO_UPDATE") {
		t.Fatalf("POLIX_NO_UPDATE: %d %q", code, stderr)
	}
}

// release builds polix as version 0.2.0 once per test run: the install scripts run the binary they install.
var release = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "polix-release-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "polix")
	if runtime.GOOS == "windows" {
		out += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", out, "-ldflags",
		"-X github.com/mlophez/polix-ai/internal/cli.Version=0.2.0", "github.com/mlophez/polix-ai/cmd/polix")
	if msg, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go build: %v\n%s", err, msg)
	}
	return os.ReadFile(out)
})

func releaseBinary(t *testing.T) []byte {
	t.Helper()
	bin, err := release()
	if err != nil {
		t.Fatal(err)
	}
	return bin
}

// runInstall runs an install script against the fake release and returns its output.
func runInstall(t *testing.T, dir string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "POLIX_INSTALL_DIR="+dir, "POLIX_RELEASES_URL="+os.Getenv("POLIX_RELEASES_URL"),
		"POLIX_NO_MODIFY_PATH=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// install.sh downloads the binary of the platform, checks it and installs it (macOS and Linux; on Windows it
// hands over to install.ps1, tested below).
func TestInstallScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on Windows install.sh runs install.ps1, which TestInstallPowerShell covers")
	}
	bin := releaseBinary(t)
	newFakeRelease(t, bin)
	dir := t.TempDir()
	script := filepath.Join("..", "..", "install.sh")
	out, err := runInstall(t, dir, "sh", script)
	if err != nil || !strings.Contains(out, "installed polix 0.2.0") || content(t, filepath.Join(dir, "polix")) != string(bin) {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}

	os.Remove(filepath.Join(dir, "polix"))
	newFakeReleaseFiles(t, map[string][]byte{asset: []byte("tampered")}, map[string][]byte{asset: bin})
	out, err = runInstall(t, dir, "sh", script)
	if err == nil || !strings.Contains(out, "checksum") {
		t.Fatalf("a tampered binary is rejected: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "polix")); err == nil {
		t.Fatal("a tampered binary must not be installed")
	}
}

// install.ps1 is the installer of Windows; it also runs on PowerShell 7 elsewhere. It is tested with every
// PowerShell found: on the Windows runners both Windows PowerShell 5.1 (powershell) and PowerShell 7 (pwsh).
func TestInstallPowerShell(t *testing.T) {
	found := false
	for _, ps := range []string{"pwsh", "powershell"} {
		if _, err := exec.LookPath(ps); err != nil {
			continue
		}
		found = true
		t.Run(ps, func(t *testing.T) { testInstallPowerShell(t, ps) })
	}
	if !found {
		t.Skip("no PowerShell on this machine")
	}
}

func testInstallPowerShell(t *testing.T, ps string) {
	bin := releaseBinary(t)
	newFakeRelease(t, bin)
	dir := t.TempDir()
	script, _ := filepath.Abs(filepath.Join("..", "..", "install.ps1"))
	args := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script}
	exe := filepath.Join(dir, "polix")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	out, err := runInstall(t, dir, ps, args...)
	if err != nil || !strings.Contains(out, "installed polix 0.2.0") || content(t, exe) != string(bin) {
		t.Fatalf("install.ps1: %v\n%s", err, out)
	}
	// Installing again replaces the binary in place (on Windows, moving the old one aside).
	out, err = runInstall(t, dir, ps, args...)
	if err != nil || !strings.Contains(out, "installed polix 0.2.0") {
		t.Fatalf("install.ps1 over an installed polix: %v\n%s", err, out)
	}

	os.Remove(exe)
	newFakeReleaseFiles(t, map[string][]byte{asset: []byte("tampered")}, map[string][]byte{asset: bin})
	out, err = runInstall(t, dir, ps, args...)
	if err == nil || !strings.Contains(out, "checksum") {
		t.Fatalf("a tampered binary is rejected: %v\n%s", err, out)
	}
	if _, err := os.Stat(exe); err == nil {
		t.Fatal("a tampered binary must not be installed")
	}
}
