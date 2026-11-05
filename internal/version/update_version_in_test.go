package version

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/pkg/cli"
)

// updateVersionFake is the port of UpdateVersion in memory: a binary, a cache, a releases page whose latest
// release is latest, and the record of what the shell asked.
type updateVersionFake struct {
	exe        string
	exeErr     error
	cache      Cache
	rejected   error
	latest     string
	latestErr  error
	installErr error
	refreshErr error
	doctorErr  error
	binary     string // the version of the binary at exe, once installed
	calls      []string
}

func (f *updateVersionFake) Executable(context.Context) (string, error) {
	f.calls = append(f.calls, "executable")
	return f.exe, f.exeErr
}

func (f *updateVersionFake) LoadCache(context.Context) Cache { return f.cache }

func (f *updateVersionFake) SaveCache(_ context.Context, c Cache) error {
	f.calls = append(f.calls, "save")
	f.cache = c
	return nil
}

func (f *updateVersionFake) WorkflowRejected(context.Context, string) error {
	f.calls = append(f.calls, "workflow")
	return f.rejected
}

func (f *updateVersionFake) Latest(context.Context) (string, error) {
	f.calls = append(f.calls, "latest")
	return f.latest, f.latestErr
}

func (f *updateVersionFake) Install(_ context.Context, exe, version string) error {
	f.calls = append(f.calls, "install "+version)
	if f.installErr == nil {
		f.binary = version
	}
	return f.installErr
}

func (f *updateVersionFake) Refresh(_ context.Context, exe string, hook bool) (string, string, error) {
	f.calls = append(f.calls, fmt.Sprintf("refresh hook=%v", hook))
	if hook {
		return "polix ha actualizado su plugin: se carga en la próxima sesión.\n", "", f.refreshErr
	}
	return "installed plugin\n", "", f.refreshErr
}

func (f *updateVersionFake) Doctor(context.Context, string) (string, string, error) {
	f.calls = append(f.calls, "doctor")
	return "polix doctor ha encontrado 1 problema\n", "", f.doctorErr
}

// asked reports whether the shell made the call.
func (f *updateVersionFake) asked(call string) bool {
	for _, c := range f.calls {
		if c == call {
			return true
		}
	}
	return false
}

var updateNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// updateRun runs the shell and serializes its response as the command does.
func updateRun(t *testing.T, f *updateVersionFake, req UpdateVersionRequest) (UpdateVersionResponse, string, string) {
	t.Helper()
	if req.Now.IsZero() {
		req.Now = updateNow
	}
	res, err := updateVersion(context.Background(), f, req)
	if err != nil {
		t.Fatalf("the shell never fails: %v", err)
	}
	var stdout, stderr bytes.Buffer
	res.print(&stdout, &stderr)
	return res, stdout.String(), stderr.String()
}

func TestUpdateVersionUpdates(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0"}
	res, stdout, _ := updateRun(t, f, UpdateVersionRequest{Check: true, Version: "0.1.0"})
	if res.Failure != "" || !strings.Contains(stdout, "can be updated to 0.2.0") || f.binary != "" ||
		f.asked("refresh hook=false") {
		t.Fatalf("--check only reports: %+v %q %v", res, stdout, f.calls)
	}
	if !f.cache.CheckedAt.IsZero() || f.cache.Latest != "0.2.0" {
		t.Fatalf("--check leaves this binary behind: it does not count as a check: %+v", f.cache)
	}

	f.calls = nil
	res, stdout, _ = updateRun(t, f, UpdateVersionRequest{Version: "0.1.0"})
	if res.Failure != "" || !strings.Contains(stdout, "polix updated from 0.1.0 to 0.2.0 (/bin/polix)") ||
		f.binary != "0.2.0" || !strings.Contains(stdout, "installed plugin") || !f.asked("refresh hook=false") {
		t.Fatalf("update: %+v %q %v", res, stdout, f.calls)
	}
	if !f.cache.CheckedAt.Equal(updateNow) || f.cache.Latest != "0.2.0" {
		t.Fatalf("an installed latest release counts as a check: %+v", f.cache)
	}
	if f.asked("doctor") {
		t.Fatal("only the hook runs the doctor")
	}

	f.calls = nil
	res, stdout, _ = updateRun(t, f, UpdateVersionRequest{Version: "0.2.0"})
	if res.Failure != "" || !strings.Contains(stdout, "polix 0.2.0 is up to date") || !f.asked("refresh hook=false") ||
		f.asked("install 0.2.0") {
		t.Fatalf("up to date: %+v %q %v", res, stdout, f.calls)
	}
}

// --version installs a given release, older ones included, and a development build is only replaced that way.
func TestUpdateVersionDevelopmentBuild(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0"}
	res, _, stderr := updateRun(t, f, UpdateVersionRequest{Version: "dev"})
	if res.Failure == "" || !strings.Contains(stderr, "development build") || f.binary != "" ||
		f.asked("refresh hook=false") {
		t.Fatalf("a dev build is not replaced by the latest: %+v %q %v", res, stderr, f.calls)
	}
	f.calls = nil
	res, _, _ = updateRun(t, f, UpdateVersionRequest{Version: "dev", Want: "0.1.0"})
	if res.Failure != "" || f.binary != "0.1.0" || f.asked("latest") || !f.asked("refresh hook=false") {
		t.Fatalf("--version: %+v %v", res, f.calls)
	}
}

// The hook mode checks at most once an hour, never fails, says nothing when there is nothing to say and ends with
// the doctor.
func TestUpdateVersionHook(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0"}
	res, stdout, _ := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"})
	if res.Failure != "" || !strings.HasPrefix(stdout, "polix se ha actualizado de 0.1.0 a 0.2.0.\n") ||
		f.binary != "0.2.0" || !f.asked("refresh hook=true") || !strings.HasSuffix(stdout, "ha encontrado 1 problema\n") {
		t.Fatalf("first hook updates, refreshes and runs the doctor: %+v %q %v", res, stdout, f.calls)
	}

	f.calls = nil
	res, stdout, _ = updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.2.0", Now: updateNow.Add(time.Minute)})
	if res.Message != "" || res.Failure != "" || f.asked("latest") || !f.asked("doctor") || !f.asked("refresh hook=true") {
		t.Fatalf("a second hook within the hour asks nothing: %+v %q %v", res, stdout, f.calls)
	}

	f.calls = nil
	f.cache = Cache{CheckedAt: updateNow.Add(-2 * time.Hour)}
	f.latestErr = errors.New("dial tcp 127.0.0.1:1: connection refused")
	res, stdout, stderr := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.2.0"})
	if res.Message != "" || !strings.Contains(stderr, "polix update: dial tcp") || !f.asked("doctor") ||
		!f.asked("refresh hook=true") || !f.cache.FailedAt.Equal(updateNow) {
		t.Fatalf("without network the hook says nothing on stdout and waits: %+v %q %q %v", res, stdout, stderr, f.calls)
	}
}

func TestUpdateVersionDisabled(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0"}
	res, _, stderr := updateRun(t, f, UpdateVersionRequest{NoUpdate: true, Version: "0.1.0"})
	if !strings.Contains(stderr, "POLIX_NO_UPDATE") || res.Failure == "" || f.asked("latest") ||
		f.asked("refresh hook=false") {
		t.Fatalf("POLIX_NO_UPDATE: %+v %q %v", res, stderr, f.calls)
	}
	f.calls = nil
	res, stdout, stderr := updateRun(t, f, UpdateVersionRequest{NoUpdate: true, Hook: true, Version: "0.1.0"})
	if res.Failure != "" || stderr != "" || !strings.Contains(stdout, "polix doctor ha encontrado") ||
		f.asked("latest") || f.asked("workflow") || f.asked("refresh hook=true") {
		t.Fatalf("POLIX_NO_UPDATE only skips the update, not the doctor: %+v %q %v", res, stdout, f.calls)
	}
}

// A download that fails leaves the check undone and the failure recorded, so the hook tries again after a while.
func TestUpdateVersionHookRetriesAFailedDownload(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0",
		installErr: errors.New("polix_linux_amd64: checksum 1 does not match 2 of checksums.txt")}
	res, _, stderr := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"})
	if res.Failure == "" || !strings.Contains(stderr, "checksum") || f.binary != "" {
		t.Fatalf("a tampered release is not installed: %+v %q", res, stderr)
	}
	if !f.cache.CheckedAt.IsZero() || f.cache.Latest != "0.2.0" || !f.cache.FailedAt.Equal(updateNow) {
		t.Fatalf("the failed check is not saved as done, the failure is: %+v", f.cache)
	}
	if !f.asked("refresh hook=true") {
		t.Fatal("a failure other than the replace still refreshes the plugin of the hook")
	}
	f.calls = nil
	if res, _, _ = updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0", Now: updateNow.Add(time.Minute)}); f.asked("latest") || res.Failure != "" {
		t.Fatalf("within the wait after a failure the hook does not try: %+v %v", res, f.calls)
	}
	res, _, stderr = updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0",
		Now: updateNow.Add(RetryAfter + time.Minute)})
	if !strings.Contains(stderr, "checksum") {
		t.Fatalf("after the wait the hook tries again: %+v %q", res, stderr)
	}
}

// A binary that cannot be replaced is not refreshed by this one: another process may have updated it.
func TestUpdateVersionReplaceFails(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0",
		installErr: fmt.Errorf("%w: rename: permission denied", ErrReplace)}
	res, _, stderr := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"})
	if !strings.Contains(stderr, "cannot replace the binary") || f.asked("refresh hook=true") || !f.asked("doctor") ||
		res.Failure == "" {
		t.Fatalf("replace failed: %+v %q %v", res, stderr, f.calls)
	}
	f.calls, f.cache = nil, Cache{}
	res, _, stderr = updateRun(t, f, UpdateVersionRequest{Version: "0.1.0"})
	if !strings.Contains(stderr, "polix update: cannot replace") || f.asked("refresh hook=false") || f.asked("doctor") {
		t.Fatalf("replace failed for a person: %+v %q %v", res, stderr, f.calls)
	}
}

// A newer release the cache already knows (polix doctor saw it) is installed at once, not within the hour; so is
// one when this binary rejects the workflow in a way a newer polix may not.
func TestUpdateVersionHookChecksAtOnce(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0", cache: Cache{CheckedAt: updateNow, Latest: "0.2.0"}}
	if _, stdout, _ := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"}); !strings.Contains(stdout, "actualizado") {
		t.Fatalf("a known newer release: %q", stdout)
	}
	f = &updateVersionFake{exe: "/bin/polix", latest: "0.2.0", cache: Cache{CheckedAt: updateNow, Latest: "0.1.0"},
		rejected: errors.New("polix.yaml requires polix 0.2.0 or newer and this is 0.1.0: run polix update")}
	if _, stdout, _ := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"}); !strings.Contains(stdout, "actualizado de 0.1.0 a 0.2.0") {
		t.Fatalf("a workflow ahead skips the hour: %q %v", stdout, f.calls)
	}
	f = &updateVersionFake{exe: "/bin/polix", latest: "0.2.0", cache: Cache{CheckedAt: updateNow, Latest: "0.1.0"},
		rejected: errors.New("polix.yaml: max_stop_blocks must not be negative")}
	if updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"}); f.asked("latest") {
		t.Fatal("a plain mistake of the workflow waits for the hour")
	}
}

// --check with --version compares the requested version with the running one and installs nothing.
func TestUpdateVersionCheckVersion(t *testing.T) {
	for v, want := range map[string]string{
		"0.3.0": "is newer than this binary",
		"0.2.0": "is already this binary",
		"0.1.0": "would be a downgrade",
	} {
		f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0"}
		_, stdout, _ := updateRun(t, f, UpdateVersionRequest{Check: true, Want: v, Version: "0.2.0"})
		if !strings.Contains(stdout, want) || f.binary != "" || f.asked("latest") {
			t.Errorf("--check --version %s: %q %v", v, stdout, f.calls)
		}
	}
}

// Without the path of the running binary nothing can be replaced nor run.
func TestUpdateVersionWithoutExecutable(t *testing.T) {
	f := &updateVersionFake{exeErr: errors.New("executable: not found")}
	res, _, stderr := updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.1.0"})
	if !strings.Contains(stderr, "polix update: executable: not found") || len(f.calls) != 1 || !res.Hook {
		t.Fatalf("%+v %q %v", res, stderr, f.calls)
	}
}

// A refresh or a doctor that fails is told, the first as a warning for a person.
func TestUpdateVersionRefreshAndDoctorFail(t *testing.T) {
	f := &updateVersionFake{exe: "/bin/polix", latest: "0.2.0", refreshErr: errors.New("exit status 1")}
	_, _, stderr := updateRun(t, f, UpdateVersionRequest{Version: "0.2.0"})
	if !strings.Contains(stderr, "polix update: the plugin was not refreshed: exit status 1") {
		t.Fatalf("refresh failed: %q", stderr)
	}
	f = &updateVersionFake{exe: "/bin/polix", latest: "0.2.0", refreshErr: errors.New("exit status 1"),
		doctorErr: errors.New("exit status 2")}
	_, _, stderr = updateRun(t, f, UpdateVersionRequest{Hook: true, Version: "0.2.0", Now: updateNow})
	if stderr != "polix update: exit status 1\npolix doctor: exit status 2\n" {
		t.Fatalf("refresh and doctor failed in the hook: %q", stderr)
	}
}

// The update runs these flags of the command; the hooks of every release run polix update --hook.
func TestUpdateVersionCommandFlags(t *testing.T) {
	c := UpdateVersionCommand()
	for _, name := range []string{"check", "version", "hook"} {
		if c.Flags().Lookup(name) == nil {
			t.Errorf("polix update has no --%s", name)
		}
	}
}

// updateCommand runs polix update with args, against the fake releases page and a cache of its own, and returns
// the exit code and the output.
func updateCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	c := UpdateVersionCommand()
	var stdout, stderr bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stderr)
	c.SetArgs(args)
	c.SilenceUsage, c.SilenceErrors = true, true
	code := 0
	if err := c.ExecuteContext(context.Background()); err != nil {
		code = 1
		var exit cli.ExitCode
		if errors.As(err, &exit) {
			code = int(exit)
		}
	}
	return code, stdout.String(), stderr.String()
}

// The entry reads the flags and POLIX_NO_UPDATE into the request; the binary of the tests is a development build.
func TestUpdateVersionCommand(t *testing.T) {
	updateReleases(t, []byte("new"), []byte("new"))
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
	t.Setenv("POLIX_NO_UPDATE", "")
	t.Chdir(t.TempDir())

	code, stdout, stderr := updateCommand(t, "--check")
	if code != 0 || !strings.Contains(stdout, "polix dev is a development build; the latest release is 0.2.0") {
		t.Fatalf("--check: %d %q %q", code, stdout, stderr)
	}
	code, stdout, _ = updateCommand(t, "--check", "--version", "v0.3.0")
	if code != 0 || !strings.Contains(stdout, "--version 0.3.0 to install 0.3.0") || strings.Contains(stdout, "v0.3.0") {
		t.Fatalf("the leading v of --version is dropped: %d %q", code, stdout)
	}

	t.Setenv("POLIX_NO_UPDATE", "1")
	if code, _, stderr = updateCommand(t); code != 1 || !strings.Contains(stderr, "POLIX_NO_UPDATE") {
		t.Fatalf("POLIX_NO_UPDATE: %d %q", code, stderr)
	}
	// The hook still runs the doctor, here the binary of the tests acting as a fake polix.
	t.Setenv(updateFakeEnv, "1")
	code, stdout, _ = updateCommand(t, "--hook")
	if code != 0 || !strings.Contains(stdout, "ran doctor --hook") {
		t.Fatalf("--hook with POLIX_NO_UPDATE never fails and runs the doctor: %d %q", code, stdout)
	}
}
