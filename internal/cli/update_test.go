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
	if runtime.GOOS == "windows" {
		exe += ".exe" // exec only runs a file with an extension of PATHEXT
	}
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe, oldVersion := executable, Version
	executable, Version = func() (string, error) { return exe, nil }, version
	t.Cleanup(func() { executable, Version = oldExe, oldVersion })
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
	t.Setenv("POLIX_NO_UPDATE", "")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	return exe
}

// outsideRepo runs the rest of the test in an empty folder: update and its hook act on the repository of the
// working folder, which must not be the one of the tests.
func outsideRepo(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
}

// withoutDoctor is what the session start hook said before the doctor it ends with.
func withoutDoctor(stdout string) string {
	before, _, _ := strings.Cut(stdout, "polix doctor ha encontrado")
	return before
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
	outsideRepo(t)
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
	outsideRepo(t)
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

// The hook mode checks at most once an hour, never fails, says nothing when there is nothing to say and ends with
// the doctor.
func TestUpdateHook(t *testing.T) {
	outsideRepo(t)
	rel := newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.1.0")

	code, stdout, _ := runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stdout, "actualizado de 0.1.0 a 0.2.0") || content(t, exe) != "new" {
		t.Fatalf("first hook updates: %d %q", code, stdout)
	}
	n := rel.requests.Load()
	Version = "0.2.0" // the binary the update left
	code, stdout, _ = runCmd("update", "--hook")
	if code != 0 || withoutDoctor(stdout) != "" || rel.requests.Load() != n {
		t.Fatalf("a second hook within the hour does nothing: %d %q, %d requests", code, stdout, rel.requests.Load()-n)
	}
	if !strings.Contains(stdout, "polix doctor ha encontrado") {
		t.Fatalf("the hook ends with the doctor, which sees the plugin is missing: %q", stdout)
	}

	if err := update.SaveCache(update.Cache{CheckedAt: time.Now().Add(-2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("POLIX_RELEASES_URL", "http://127.0.0.1:1") // nothing listens
	code, stdout, _ = runCmd("update", "--hook")
	if code != 0 || withoutDoctor(stdout) != "" {
		t.Fatalf("without network the hook stays quiet: %d %q", code, stdout)
	}

	t.Setenv("POLIX_NO_UPDATE", "1")
	if code, _, stderr := runCmd("update"); code != 1 || !strings.Contains(stderr, "POLIX_NO_UPDATE") {
		t.Fatalf("POLIX_NO_UPDATE: %d %q", code, stderr)
	}
	if code, stdout, _ := runCmd("update", "--hook"); code != 0 || !strings.Contains(stdout, "polix doctor ha encontrado") {
		t.Fatalf("POLIX_NO_UPDATE only skips the update, not the doctor: %d %q", code, stdout)
	}
}

// A download that fails leaves the check undone, so the next session start tries again instead of within the hour.
func TestUpdateHookRetriesAFailedDownload(t *testing.T) {
	outsideRepo(t)
	newFakeReleaseFiles(t, map[string][]byte{asset: []byte("new")}, map[string][]byte{asset: []byte("tampered")})
	exe := fakeBinary(t, "0.1.0")
	code, _, stderr := runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stderr, "checksum") || content(t, exe) != "old" {
		t.Fatalf("a tampered release is not installed: %d %q", code, stderr)
	}
	c := update.LoadCache()
	if !c.CheckedAt.IsZero() || c.Latest != "0.2.0" || c.FailedAt.IsZero() {
		t.Fatalf("the failed check is not saved as done, the failure is: %+v", c)
	}
	if _, _, stderr = runCmd("update", "--hook"); stderr != "" {
		t.Fatalf("within the wait after a failure the hook does not try: %q", stderr)
	}
	c.FailedAt = time.Now().Add(-retryAfter - time.Minute)
	if err := update.SaveCache(c); err != nil {
		t.Fatal(err)
	}
	if _, _, stderr = runCmd("update", "--hook"); !strings.Contains(stderr, "checksum") {
		t.Fatalf("after the wait the hook tries again: %q", stderr)
	}
	if _, _, _ = runCmd("update", "--check"); !update.LoadCache().CheckedAt.IsZero() {
		t.Fatal("--check leaves this binary behind: it does not count as a check of the hook")
	}
}

// A binary in a folder it cannot write fails before the download, and waits before trying again.
func TestUpdateHookCannotWrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only folder does not stop writes on Windows nor for root")
	}
	outsideRepo(t)
	rel := newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.1.0")
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	code, _, stderr := runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stderr, "cannot write") || content(t, exe) != "old" {
		t.Fatalf("cannot write: %d %q", code, stderr)
	}
	n := rel.requests.Load()
	if n != 1 {
		t.Fatalf("only the latest release is asked, nothing is downloaded: %d requests", n)
	}
	runCmd("update", "--hook")
	if rel.requests.Load() != n {
		t.Fatal("within the wait after a failure the hook does not ask again")
	}
}

// A newer release the cache already knows (polix doctor saw it) is installed at once, not within the hour.
func TestUpdateHookKnowsANewerRelease(t *testing.T) {
	outsideRepo(t)
	newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.1.0")
	if err := update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	if _, stdout, _ := runCmd("update", "--hook"); !strings.Contains(stdout, "actualizado") || content(t, exe) != "new" {
		t.Fatalf("a known newer release: %q", stdout)
	}
}

// The doctor of the hook is the one of the binary the update left: a real release, run after the replace.
func TestUpdateHookRunsTheNewDoctor(t *testing.T) {
	outsideRepo(t)
	newFakeRelease(t, releaseBinary(t))
	exe := fakeBinary(t, "0.1.0")
	code, stdout, stderr := runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stdout, "actualizado de 0.1.0 a 0.2.0") || stderr != "" {
		t.Fatalf("update: %d %q %q", code, stdout, stderr)
	}
	out, err := exec.Command(exe, "version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "0.2.0" {
		t.Fatalf("the new binary: %q %v", out, err)
	}
	if !strings.Contains(withoutDoctor(stdout), "actualizado") || !strings.Contains(stdout, "polix doctor ha encontrado") {
		t.Fatalf("the new binary runs the doctor after the update: %q", stdout)
	}
}

// polix doctor saves the time of the check only when this binary is the latest, like the hook.
func TestDoctorCache(t *testing.T) {
	outsideRepo(t)
	newFakeRelease(t, []byte("new"))
	fakeBinary(t, "0.1.0")
	runCmd("doctor")
	if c := update.LoadCache(); c.Latest != "0.2.0" || !c.CheckedAt.IsZero() {
		t.Fatalf("behind: %+v", c)
	}
	Version = "0.2.0"
	runCmd("doctor")
	if c := update.LoadCache(); c.CheckedAt.IsZero() {
		t.Fatalf("latest: %+v", c)
	}
}

// Every release runs these commands of the next one: an old binary that updates runs them with the new binary,
// so they must keep existing with these flags (see docs/polix.md).
func TestCommandsTheUpdateRuns(t *testing.T) {
	for _, args := range [][]string{{"__refresh", "--quiet"}, {"__refresh", "--hooks"}, {"doctor", "--hook"},
		{"install", "--user", "--yes", "--quiet"}, {"update", "--hook"}, {"version"}} {
		if code, _, stderr := runCmd(append(args, "--help")...); code != 0 {
			t.Errorf("polix %s: %d %q", strings.Join(args, " "), code, stderr)
		}
	}
}

// A nested option, or a later version of the schema, may come from a newer polix too; a plain mistake may not.
func TestWorkflowAhead(t *testing.T) {
	fakeBinary(t, "0.1.0")
	r := newRepo(t, "main")
	t.Chdir(r.dir)
	for src, want := range map[string]bool{
		"version: 1\nrequires: 0.2.0\n":                                     true,
		"version: 1\nan_option: true\n":                                     true,
		"version: 1\non:\n  pre_edit:\n    - deny: x\n      an_option: 1\n": true,
		"version: 2\n":                      true,
		"version: 1\nmax_stop_blocks: -1\n": false,
		"version: 1\n":                      false,
	} {
		r.write("polix.yaml", src)
		if got := workflowAhead(); got != want {
			t.Errorf("workflowAhead with %q = %v, want %v", src, got, want)
		}
	}
}

// A repository whose workflow this binary rejects (it requires a newer polix) is checked at every session start.
func TestUpdateHookWhenTheWorkflowIsAhead(t *testing.T) {
	rel := newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.1.0")
	r := newRepo(t, "main")
	r.write("polix.yaml", "version: 1\nrequires: 0.2.0\n")
	t.Chdir(r.dir)
	if err := update.SaveCache(update.Cache{CheckedAt: time.Now(), Latest: "0.1.0"}); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCmd("update", "--hook")
	if code != 0 || !strings.Contains(stdout, "actualizado de 0.1.0 a 0.2.0") || content(t, exe) != "new" {
		t.Fatalf("a workflow ahead skips the hour: %d %q %q (%d requests)", code, stdout, stderr, rel.requests.Load())
	}
	if _, err := os.Stat(filepath.Join(r.dir, ".claude")); err == nil {
		t.Fatal("the hook never touches the files of the repository")
	}
}

// moduleDir is the root of the module, taken before any test changes the working folder.
var moduleDir, _ = filepath.Abs(filepath.Join("..", ".."))

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
	cmd.Dir = moduleDir
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

// --check with --version compares the requested version with the running one and downloads nothing.
func TestUpdateCheckVersion(t *testing.T) {
	outsideRepo(t)
	newFakeRelease(t, []byte("new"))
	exe := fakeBinary(t, "0.2.0")
	for v, want := range map[string]string{
		"0.3.0": "is newer than this binary",
		"0.2.0": "is already this binary",
		"0.1.0": "would be a downgrade",
	} {
		code, stdout, stderr := runCmd("update", "--check", "--version", v)
		if code != 0 || !strings.Contains(stdout, want) || content(t, exe) != "old" {
			t.Errorf("--check --version %s: %d %q %q", v, code, stdout, stderr)
		}
	}
}
