package update

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
	workflowload "github.com/mlophez/polix-ai/internal/app/workflow/load"
	"github.com/mlophez/polix-ai/internal/platform/cli"
)

// fakePort is the port of UpdateVersion in memory: a binary, a cache, a releases page whose latest
// release is latest, and the record of what the shell asked.
type fakePort struct {
	exe        string
	exeErr     error
	cache      version.Cache
	rejected   error
	latest     string
	latestErr  error
	installErr error
	refreshErr error
	doctorErr  error
	binary     string // the version of the binary at exe, once installed
	calls      []string
}

func (f *fakePort) Executable(context.Context) (string, error) {
	f.calls = append(f.calls, "executable")
	return f.exe, f.exeErr
}

func (f *fakePort) LoadCache(context.Context) version.Cache { return f.cache }

func (f *fakePort) SaveCache(_ context.Context, c version.Cache) error {
	f.calls = append(f.calls, "save")
	f.cache = c
	return nil
}

func (f *fakePort) WorkflowRejected(context.Context, string) error {
	f.calls = append(f.calls, "workflow")
	return f.rejected
}

func (f *fakePort) Latest(context.Context) (string, error) {
	f.calls = append(f.calls, "latest")
	return f.latest, f.latestErr
}

func (f *fakePort) Install(_ context.Context, _, release string) error {
	f.calls = append(f.calls, "install "+release)
	if f.installErr == nil {
		f.binary = release
	}
	return f.installErr
}

func (f *fakePort) Refresh(_ context.Context, _ string, hook bool) (string, string, error) {
	f.calls = append(f.calls, fmt.Sprintf("refresh hook=%v", hook))
	if hook {
		return "polix updated its plugin: it loads in the next session.\n", "", f.refreshErr
	}
	return "installed plugin\n", "", f.refreshErr
}

func (f *fakePort) Doctor(context.Context, string) (string, string, error) {
	f.calls = append(f.calls, "doctor")
	return "polix doctor found 1 problem\n", "", f.doctorErr
}

// asked reports whether the shell made the call.
func (f *fakePort) asked(call string) bool {
	return slices.Contains(f.calls, call)
}

var now = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// runShell runs the shell and serializes its response as the command does.
func runShell(t *testing.T, f *fakePort, req Request) (Response, string, string) {
	t.Helper()
	if req.Now.IsZero() {
		req.Now = now
	}
	res, err := run(context.Background(), f, req)
	if err != nil {
		t.Fatalf("the shell never fails: %v", err)
	}
	var stdout, stderr bytes.Buffer
	res.print(&stdout, &stderr)
	return res, stdout.String(), stderr.String()
}

func TestUpdates(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0"}
	res, stdout, _ := runShell(t, f, Request{Check: true, Version: "0.1.0"})
	if res.Failure != "" || !strings.Contains(stdout, "can be updated to 0.2.0") || f.binary != "" ||
		f.asked("refresh hook=false") {
		t.Fatalf("--check only reports: %+v %q %v", res, stdout, f.calls)
	}
	if !f.cache.CheckedAt.IsZero() || f.cache.Latest != "0.2.0" {
		t.Fatalf("--check leaves this binary behind: it does not count as a check: %+v", f.cache)
	}

	f.calls = nil
	res, stdout, _ = runShell(t, f, Request{Version: "0.1.0"})
	if res.Failure != "" || !strings.Contains(stdout, "polix updated from 0.1.0 to 0.2.0 (/bin/polix)") ||
		f.binary != "0.2.0" || !strings.Contains(stdout, "installed plugin") || !f.asked("refresh hook=false") {
		t.Fatalf("update: %+v %q %v", res, stdout, f.calls)
	}
	if !f.cache.CheckedAt.Equal(now) || f.cache.Latest != "0.2.0" {
		t.Fatalf("an installed latest release counts as a check: %+v", f.cache)
	}
	if f.asked("doctor") {
		t.Fatal("only the hook runs the doctor")
	}

	f.calls = nil
	res, stdout, _ = runShell(t, f, Request{Version: "0.2.0"})
	if res.Failure != "" || !strings.Contains(stdout, "polix 0.2.0 is up to date") || !f.asked("refresh hook=false") ||
		f.asked("install 0.2.0") {
		t.Fatalf("up to date: %+v %q %v", res, stdout, f.calls)
	}
}

// --version installs a given release, older ones included, and a development build is only replaced that way.
func TestDevelopmentBuild(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0"}
	res, _, stderr := runShell(t, f, Request{Version: "dev"})
	if res.Failure == "" || !strings.Contains(stderr, "development build") || f.binary != "" ||
		f.asked("refresh hook=false") {
		t.Fatalf("a dev build is not replaced by the latest: %+v %q %v", res, stderr, f.calls)
	}
	f.calls = nil
	res, _, _ = runShell(t, f, Request{Version: "dev", Want: "0.1.0"})
	if res.Failure != "" || f.binary != "0.1.0" || f.asked("latest") || !f.asked("refresh hook=false") {
		t.Fatalf("--version: %+v %v", res, f.calls)
	}
}

// The hook mode checks at most once an hour, never fails, says nothing when there is nothing to say and ends with
// the doctor.
func TestHook(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0"}
	res, stdout, _ := runShell(t, f, Request{Hook: true, Version: "0.1.0"})
	if res.Failure != "" || !strings.HasPrefix(stdout, "polix updated from 0.1.0 to 0.2.0.\n") ||
		f.binary != "0.2.0" || !f.asked("refresh hook=true") || !strings.HasSuffix(stdout, "found 1 problem\n") {
		t.Fatalf("first hook updates, refreshes and runs the doctor: %+v %q %v", res, stdout, f.calls)
	}

	f.calls = nil
	res, stdout, _ = runShell(t, f, Request{Hook: true, Version: "0.2.0", Now: now.Add(time.Minute)})
	if res.Message != "" || res.Failure != "" || f.asked("latest") || !f.asked("doctor") || !f.asked("refresh hook=true") {
		t.Fatalf("a second hook within the hour asks nothing: %+v %q %v", res, stdout, f.calls)
	}

	f.calls = nil
	f.cache = version.Cache{CheckedAt: now.Add(-2 * time.Hour)}
	f.latestErr = errors.New("dial tcp 127.0.0.1:1: connection refused")
	res, stdout, stderr := runShell(t, f, Request{Hook: true, Version: "0.2.0"})
	if res.Message != "" || !strings.Contains(stderr, "polix update: dial tcp") || !f.asked("doctor") ||
		!f.asked("refresh hook=true") || !f.cache.FailedAt.Equal(now) {
		t.Fatalf("without network the hook says nothing on stdout and waits: %+v %q %q %v", res, stdout, stderr, f.calls)
	}
}

func TestDisabled(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0"}
	res, _, stderr := runShell(t, f, Request{NoUpdate: true, Version: "0.1.0"})
	if !strings.Contains(stderr, "POLIX_NO_UPDATE") || res.Failure == "" || f.asked("latest") ||
		f.asked("refresh hook=false") {
		t.Fatalf("POLIX_NO_UPDATE: %+v %q %v", res, stderr, f.calls)
	}
	f.calls = nil
	res, stdout, stderr := runShell(t, f, Request{NoUpdate: true, Hook: true, Version: "0.1.0"})
	if res.Failure != "" || stderr != "" || !strings.Contains(stdout, "polix doctor found") ||
		f.asked("latest") || f.asked("workflow") || f.asked("refresh hook=true") {
		t.Fatalf("POLIX_NO_UPDATE only skips the update, not the doctor: %+v %q %v", res, stdout, f.calls)
	}
}

// A download that fails leaves the check undone and the failure recorded, so the hook tries again after a while.
func TestHookRetriesAFailedDownload(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0",
		installErr: errors.New("polix_linux_amd64: checksum 1 does not match 2 of checksums.txt")}
	res, _, stderr := runShell(t, f, Request{Hook: true, Version: "0.1.0"})
	if res.Failure == "" || !strings.Contains(stderr, "checksum") || f.binary != "" {
		t.Fatalf("a tampered release is not installed: %+v %q", res, stderr)
	}
	if !f.cache.CheckedAt.IsZero() || f.cache.Latest != "0.2.0" || !f.cache.FailedAt.Equal(now) {
		t.Fatalf("the failed check is not saved as done, the failure is: %+v", f.cache)
	}
	if !f.asked("refresh hook=true") {
		t.Fatal("a failure other than the replace still refreshes the plugin of the hook")
	}
	f.calls = nil
	if res, _, _ = runShell(t, f, Request{Hook: true, Version: "0.1.0", Now: now.Add(time.Minute)}); f.asked("latest") || res.Failure != "" {
		t.Fatalf("within the wait after a failure the hook does not try: %+v %v", res, f.calls)
	}
	res, _, stderr = runShell(t, f, Request{Hook: true, Version: "0.1.0",
		Now: now.Add(version.RetryAfter + time.Minute)})
	if !strings.Contains(stderr, "checksum") {
		t.Fatalf("after the wait the hook tries again: %+v %q", res, stderr)
	}
}

// A binary that cannot be replaced is not refreshed by this one: another process may have updated it.
func TestReplaceFails(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0",
		installErr: fmt.Errorf("%w: rename: permission denied", version.ErrReplace)}
	res, _, stderr := runShell(t, f, Request{Hook: true, Version: "0.1.0"})
	if !strings.Contains(stderr, "cannot replace the binary") || f.asked("refresh hook=true") || !f.asked("doctor") ||
		res.Failure == "" {
		t.Fatalf("replace failed: %+v %q %v", res, stderr, f.calls)
	}
	f.calls, f.cache = nil, version.Cache{}
	res, _, stderr = runShell(t, f, Request{Version: "0.1.0"})
	if !strings.Contains(stderr, "polix update: cannot replace") || f.asked("refresh hook=false") || f.asked("doctor") {
		t.Fatalf("replace failed for a person: %+v %q %v", res, stderr, f.calls)
	}
}

// A newer release the cache already knows (polix doctor saw it) is installed at once, not within the hour; so is
// one when this binary rejects the workflow in a way a newer polix may not.
func TestHookChecksAtOnce(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0", cache: version.Cache{CheckedAt: now, Latest: "0.2.0"}}
	if _, stdout, _ := runShell(t, f, Request{Hook: true, Version: "0.1.0"}); !strings.Contains(stdout, "updated from") {
		t.Fatalf("a known newer release: %q", stdout)
	}
	f = &fakePort{exe: "/bin/polix", latest: "0.2.0", cache: version.Cache{CheckedAt: now, Latest: "0.1.0"},
		rejected: errors.New("polix.yaml requires polix 0.2.0 or newer and this is 0.1.0: run polix update")}
	if _, stdout, _ := runShell(t, f, Request{Hook: true, Version: "0.1.0"}); !strings.Contains(stdout, "updated from 0.1.0 to 0.2.0") {
		t.Fatalf("a workflow ahead skips the hour: %q %v", stdout, f.calls)
	}
	f = &fakePort{exe: "/bin/polix", latest: "0.2.0", cache: version.Cache{CheckedAt: now, Latest: "0.1.0"},
		rejected: errors.New("polix.yaml: max_stop_blocks must not be negative")}
	if runShell(t, f, Request{Hook: true, Version: "0.1.0"}); f.asked("latest") {
		t.Fatal("a plain mistake of the workflow waits for the hour")
	}
}

// --check with --version compares the requested version with the running one and installs nothing.
func TestCheckVersion(t *testing.T) {
	for v, want := range map[string]string{
		"0.3.0": "is newer than this binary",
		"0.2.0": "is already this binary",
		"0.1.0": "would be a downgrade",
	} {
		f := &fakePort{exe: "/bin/polix", latest: "0.2.0"}
		_, stdout, _ := runShell(t, f, Request{Check: true, Want: v, Version: "0.2.0"})
		if !strings.Contains(stdout, want) || f.binary != "" || f.asked("latest") {
			t.Errorf("--check --version %s: %q %v", v, stdout, f.calls)
		}
	}
}

// Without the path of the running binary nothing can be replaced nor run.
func TestWithoutExecutable(t *testing.T) {
	f := &fakePort{exeErr: errors.New("executable: not found")}
	res, _, stderr := runShell(t, f, Request{Hook: true, Version: "0.1.0"})
	if !strings.Contains(stderr, "polix update: executable: not found") || len(f.calls) != 1 || !res.Hook {
		t.Fatalf("%+v %q %v", res, stderr, f.calls)
	}
}

// A refresh or a doctor that fails is told, the first as a warning for a person.
func TestRefreshAndDoctorFail(t *testing.T) {
	f := &fakePort{exe: "/bin/polix", latest: "0.2.0", refreshErr: errors.New("exit status 1")}
	_, _, stderr := runShell(t, f, Request{Version: "0.2.0"})
	if !strings.Contains(stderr, "polix update: the plugin was not refreshed: exit status 1") {
		t.Fatalf("refresh failed: %q", stderr)
	}
	f = &fakePort{exe: "/bin/polix", latest: "0.2.0", refreshErr: errors.New("exit status 1"),
		doctorErr: errors.New("exit status 2")}
	_, _, stderr = runShell(t, f, Request{Hook: true, Version: "0.2.0", Now: now})
	if stderr != "polix update: exit status 1\npolix doctor: exit status 2\n" {
		t.Fatalf("refresh and doctor failed in the hook: %q", stderr)
	}
}

// noCache is the loader of the cache of these tests: the cache folder of each test is empty, so it loads an empty
// cache, as the loader of internal/app/version/load would.
func noCache(context.Context, string) version.Loaded { return version.Loaded{} }

// The update runs these flags of the command; the hooks of every release run polix update --hook.
func TestCommandFlags(t *testing.T) {
	c := Command(noCache, workflowload.Command())
	for _, name := range []string{"check", "version", "hook"} {
		if c.Flags().Lookup(name) == nil {
			t.Errorf("polix update has no --%s", name)
		}
	}
}

// runCommand runs polix update with args, against the fake releases page and a cache of its own, and returns
// the exit code and the output.
func runCommand(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	c := Command(noCache, workflowload.Command())
	var stdout, stderr bytes.Buffer
	c.SetOut(&stdout)
	c.SetErr(&stderr)
	c.SetArgs(args)
	c.SilenceUsage, c.SilenceErrors = true, true
	code := 0
	if err := c.ExecuteContext(context.Background()); err != nil {
		code = 1
		if exit, ok := errors.AsType[cli.ExitCodeError](err); ok {
			code = int(exit)
		}
	}
	return code, stdout.String(), stderr.String()
}

// The entry reads the flags and POLIX_NO_UPDATE into the request; the binary of the tests is a development build.
func TestCommand(t *testing.T) {
	serveReleases(t, []byte("new"), []byte("new"))
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
	t.Setenv("POLIX_NO_UPDATE", "")
	t.Chdir(t.TempDir())

	code, stdout, stderr := runCommand(t, "--check")
	if code != 0 || !strings.Contains(stdout, "polix dev is a development build; the latest release is 0.2.0") {
		t.Fatalf("--check: %d %q %q", code, stdout, stderr)
	}
	code, stdout, _ = runCommand(t, "--check", "--version", "v0.3.0")
	if code != 0 || !strings.Contains(stdout, "--version 0.3.0 to install 0.3.0") || strings.Contains(stdout, "v0.3.0") {
		t.Fatalf("the leading v of --version is dropped: %d %q", code, stdout)
	}

	t.Setenv("POLIX_NO_UPDATE", "1")
	if code, _, stderr = runCommand(t); code != 1 || !strings.Contains(stderr, "POLIX_NO_UPDATE") {
		t.Fatalf("POLIX_NO_UPDATE: %d %q", code, stderr)
	}
	// The hook still runs the doctor, here the binary of the tests acting as a fake polix.
	t.Setenv(fakeEnv, "1")
	code, stdout, _ = runCommand(t, "--hook")
	if code != 0 || !strings.Contains(stdout, "ran doctor --hook") {
		t.Fatalf("--hook with POLIX_NO_UPDATE never fails and runs the doctor: %d %q", code, stdout)
	}
}
