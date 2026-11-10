// Package install tests install.sh, install.ps1 and the update of a real release binary end to end: they build
// polix with the version flag of a release and serve it from a fake releases page.
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/github"
)

// asset is the name of the binary of this platform in a release.
var asset = github.AssetName("polix", runtime.GOOS, runtime.GOARCH)

// newFakeRelease serves bin as the binary of this platform.
func newFakeRelease(t *testing.T, bin []byte) {
	newFakeReleaseFiles(t, map[string][]byte{asset: bin}, nil)
}

// newFakeReleaseFiles serves a releases page whose latest release, v0.2.0, holds files with their checksums.txt;
// sums overrides the checksum of some of them, to serve a tampered release.
func newFakeReleaseFiles(t *testing.T, files map[string][]byte, sums map[string][]byte) {
	t.Helper()
	var listing strings.Builder
	for name, data := range files {
		if fake, ok := sums[name]; ok {
			data = fake
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&listing, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v0.2.0", http.StatusFound)
	})
	for _, prefix := range []string{"/download/v0.2.0/", "/latest/download/"} {
		mux.HandleFunc(prefix+"checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(listing.String()))
		})
		for name, data := range files {
			mux.HandleFunc(prefix+name, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(data) })
		}
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
}

func content(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// moduleDir is the root of the module, taken before any test changes the working folder.
var moduleDir, _ = filepath.Abs(filepath.Join("..", ".."))

// buildRelease builds polix as a release of version, with the linker flag of scripts/dist.sh.
func buildRelease(version string) ([]byte, error) {
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
		"-X github.com/mlophez/polix-ai/internal/platform/build.Version="+version, "github.com/mlophez/polix-ai/cmd/polix")
	cmd.Dir = moduleDir
	if msg, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go build: %w\n%s", err, msg)
	}
	return os.ReadFile(out)
}

// release is polix 0.2.0, built once per test run: the install scripts run the binary they install.
var release = sync.OnceValues(func() ([]byte, error) { return buildRelease("0.2.0") })

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
	// The installed binary runs polix install: without the CLIs of the agents on its PATH, and with a config
	// folder of Claude Code of its own, it cannot change the plugins of the machine.
	cmd.Env = append(os.Environ(), "POLIX_INSTALL_DIR="+dir, "POLIX_RELEASES_URL="+os.Getenv("POLIX_RELEASES_URL"),
		"POLIX_NO_MODIFY_PATH=1", "PATH="+pathWithoutAgents(), "CLAUDE_CONFIG_DIR="+t.TempDir())
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// pathWithoutAgents is the PATH without the folders that hold the CLI of Claude Code or Codex.
func pathWithoutAgents() string {
	var keep []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		agent := false
		for _, name := range []string{"claude", "claude.exe", "claude.cmd", "codex", "codex.exe", "codex.cmd"} {
			if _, err := os.Stat(filepath.Join(d, name)); err == nil {
				agent = true
			}
		}
		if !agent {
			keep = append(keep, d)
		}
	}
	return strings.Join(keep, string(os.PathListSeparator))
}

// install.sh downloads the binary of the platform, checks it and installs it (macOS and Linux; on Windows it
// hands over to install.ps1, tested below).
func TestInstallScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("on Windows install.sh runs install.ps1, which TestInstallPowerShell covers")
	}
	bin := releaseBinary(t)
	newFakeRelease(t, bin)
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	script := filepath.Join("..", "..", "install.sh")
	out, err := runInstall(t, dir, "sh", script)
	if err != nil || !strings.Contains(out, "installed polix 0.2.0") || content(t, filepath.Join(dir, "polix")) != string(bin) {
		t.Fatalf("install.sh: %v\n%s", err, out)
	}
	if v, ok := pluginVersion(); !ok || v != "0.2.0" || !strings.Contains(out, "/polix:add-rule") {
		t.Fatalf("install.sh runs polix install: %q %v\n%s", v, ok, out)
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

	newFakeRelease(t, bin)
	if out, err = runInstall(t, dir, "sh", script); err != nil {
		t.Fatalf("first install: %v\n%s", err, out)
	}
	if out, err = runInstall(t, dir, "sh", script); err != nil || content(t, filepath.Join(dir, "polix")) != string(bin) {
		t.Fatalf("install over an installed polix: %v\n%s", err, out)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("only polix is left in its folder: %v", entries)
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
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	if v, ok := pluginVersion(); !ok || v != "0.2.0" {
		t.Fatalf("install.ps1 runs polix install: %q %v\n%s", v, ok, out)
	}
	// Installing again replaces the binary in place (on Windows, moving the old one aside).
	out, err = runInstall(t, dir, ps, args...)
	if err != nil || !strings.Contains(out, "installed polix 0.2.0") {
		t.Fatalf("install.ps1 over an installed polix: %v\n%s", err, out)
	}

	// Run through iex, as `irm | iex` does, the script leaves none of its variables in the session.
	cmd := fmt.Sprintf("Get-Content -Raw '%s' | Invoke-Expression; if (Test-Path variable:version) { 'LEAKED' } else { 'CLEAN' }",
		strings.ReplaceAll(script, "'", "''"))
	out, err = runInstall(t, dir, ps, "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmd)
	if err != nil || !strings.Contains(out, "CLEAN") {
		t.Fatalf("install.ps1 through iex leaves its variables: %v\n%s", err, out)
	}

	if runtime.GOOS == "windows" {
		testInstallPowerShellPath(t, ps, dir, args)
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

// userPathPS reads the user Path of the registry unexpanded, and its type, as "<kind>|<value>".
const userPathPS = `$k = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment'); ` +
	`$v = $k.GetValue('Path', $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames); ` +
	`if ($null -eq $v) { 'None|' } else { "$($k.GetValueKind('Path'))|$v" }`

// testInstallPowerShellPath checks on Windows that install.ps1 adds its folder to the user Path keeping the
// entries with variables and the type of the value, and restores the Path of the machine afterwards.
func testInstallPowerShellPath(t *testing.T, ps, dir string, args []string) {
	t.Helper()
	runPS := func(script string) string {
		t.Helper()
		out, err := exec.Command(ps, "-NoProfile", "-Command", script).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v\n%s", script, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	kind, orig, _ := strings.Cut(runPS(userPathPS), "|")
	t.Cleanup(func() {
		if kind == "None" {
			runPS(`[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true).DeleteValue('Path', $false)`)
			return
		}
		runPS(fmt.Sprintf(`[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true).SetValue('Path', '%s', [Microsoft.Win32.RegistryValueKind]::%s)`,
			strings.ReplaceAll(orig, "'", "''"), kind))
	})
	entry := `%USERPROFILE%\polix-test-entry`
	for _, want := range []string{"ExpandString", "String"} {
		runPS(fmt.Sprintf(`[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true).SetValue('Path', '%s', [Microsoft.Win32.RegistryValueKind]::%s)`,
			strings.ReplaceAll(entry+";"+orig, "'", "''"), want))

		cmd := exec.Command(ps, args...)
		cmd.Env = append(os.Environ(), "POLIX_INSTALL_DIR="+dir, "POLIX_RELEASES_URL="+os.Getenv("POLIX_RELEASES_URL"),
			"POLIX_NO_SETUP=1", "PATH="+pathWithoutAgents())
		if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "added "+dir) {
			t.Fatalf("install.ps1 adds its folder to the user Path (%s): %v\n%s", want, err, out)
		}
		gotKind, got, _ := strings.Cut(runPS(userPathPS), "|")
		entries := strings.Split(got, ";")
		if gotKind != want || entries[0] != entry || entries[len(entries)-1] != dir {
			t.Fatalf("the user Path keeps %s and its type %s, and ends with %s: %s %q", entry, want, dir, gotKind, got)
		}
	}
}

// pluginVersion returns the version of the plugin polix install wrote in the marketplace of the home of the test.
func pluginVersion() (string, bool) {
	data, err := os.ReadFile(filepath.Join(os.Getenv("HOME"), ".polix", "marketplace", "polix", ".claude-plugin", "plugin.json"))
	var m struct{ Name, Version string }
	if err != nil || json.Unmarshal(data, &m) != nil || m.Name != "polix" {
		return "", false
	}
	return m.Version, true
}

// A release binary that polix update --hook replaces reports the new version, and the new binary runs its doctor.
func TestUpdateHookRunsTheNewDoctor(t *testing.T) {
	old, err := buildRelease("0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	newFakeRelease(t, releaseBinary(t))
	exe := filepath.Join(t.TempDir(), "polix")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if err := os.WriteFile(exe, old, 0o755); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	cmd := exec.Command(exe, "update", "--hook")
	cmd.Dir = t.TempDir() // the update acts on the repository of the working folder, which must not be this one
	cmd.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "POLIX_CACHE_DIR="+t.TempDir(),
		"POLIX_NO_UPDATE=", "PATH="+pathWithoutAgents(), "CLAUDE_CONFIG_DIR="+t.TempDir())
	out, err := cmd.Output()
	stdout := string(out)
	if err != nil || !strings.Contains(stdout, "actualizado de 0.1.0 a 0.2.0") {
		t.Fatalf("update --hook: %v %q", err, stdout)
	}
	if !strings.Contains(stdout, "polix doctor found problems") {
		t.Fatalf("the new binary runs the doctor after the update: %q", stdout)
	}
	if v, err := exec.Command(exe, "version").Output(); err != nil || strings.TrimSpace(string(v)) != "0.2.0" {
		t.Fatalf("the new binary: %q %v", v, err)
	}
}
