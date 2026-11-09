package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/version"
	"github.com/mlophez/polix-ai/internal/platform/github"
)

// fakeEnv makes the test binary act as a fake polix that prints its arguments, so the adapter runs a real
// process on every system; with fakeFail it also fails.
const (
	fakeEnv  = "POLIX_TEST_FAKE_POLIX"
	fakeFail = "POLIX_TEST_FAKE_FAIL"
)

// TestMain runs the fake polix when the adapter starts the test binary as one.
func TestMain(m *testing.M) {
	if os.Getenv(fakeEnv) != "" {
		wd, _ := os.Getwd()
		fmt.Printf("ran %s in %s\n", strings.Join(os.Args[1:], " "), filepath.Base(wd))
		fmt.Fprintln(os.Stderr, "to stderr")
		if os.Getenv(fakeFail) != "" {
			os.Exit(3)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// asset is the name of the binary of this system in a release.
var asset = github.AssetName("polix", runtime.GOOS, runtime.GOARCH)

// serveReleases serves a releases page whose latest release, v0.2.0, holds bin with the checksum of sum, and
// counts the requests; it points POLIX_RELEASES_URL at it.
func serveReleases(t *testing.T, bin, sum []byte) *atomic.Int32 {
	t.Helper()
	var requests atomic.Int32
	digest := sha256.Sum256(sum)
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v0.2.0", http.StatusFound)
	})
	mux.HandleFunc("/download/v0.2.0/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "0000  polix_other_os\n%s  %s\n", hex.EncodeToString(digest[:]), asset)
	})
	// The binary is served through a redirect, as GitHub does.
	mux.HandleFunc("/download/v0.2.0/"+asset, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/"+asset, http.StatusFound)
	})
	mux.HandleFunc("/assets/"+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bin) })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	return &requests
}

// binary writes an old binary in a temporary folder.
func binary(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	return exe
}

func content(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestAdapterLatestAndInstall(t *testing.T) {
	requests := serveReleases(t, []byte("new"), []byte("new"))
	a := Adapter{Dir: t.TempDir()}
	latest, err := a.Latest(context.Background())
	if err != nil || latest != "0.2.0" {
		t.Fatalf("Latest = %q, %v", latest, err)
	}
	exe := binary(t)
	if err := a.Install(context.Background(), exe, "0.2.0"); err != nil || content(t, exe) != "new" {
		t.Fatalf("Install: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) > 2 { // the binary, and on Windows the old one
		t.Fatalf("temporary files left: %v", entries)
	}
	if requests.Load() == 0 {
		t.Fatal("the fake releases page was not asked")
	}
}

// A binary that does not match checksums.txt is never installed.
func TestAdapterRejectsTamperedBinary(t *testing.T) {
	serveReleases(t, []byte("tampered"), []byte("original"))
	exe := binary(t)
	err := Adapter{}.Install(context.Background(), exe, "0.2.0")
	if err == nil || !strings.Contains(err.Error(), "checksum") || content(t, exe) != "old" {
		t.Fatalf("want a checksum error and the old binary, got %v", err)
	}
	if errors.Is(err, version.ErrReplace) {
		t.Fatal("a failed download is not a failed replace")
	}
}

// A binary in a folder it cannot write fails before the download.
func TestAdapterCannotWrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("a read-only folder does not stop writes on Windows nor for root")
	}
	requests := serveReleases(t, []byte("new"), []byte("new"))
	exe := binary(t)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	err := Adapter{}.Install(context.Background(), exe, "0.2.0")
	if err == nil || !strings.Contains(err.Error(), "cannot write") || requests.Load() != 0 {
		t.Fatalf("cannot write: %v, %d requests", err, requests.Load())
	}
}

// While a repository has no release, GitHub sends releases/latest back to the releases page; without network the
// hook gives up.
func TestAdapterLatestFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	}))
	defer srv.Close()
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	if _, err := (Adapter{}).Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no release") {
		t.Fatalf("want no release, got %v", err)
	}
	t.Setenv("POLIX_RELEASES_URL", "http://127.0.0.1:1") // nothing listens
	if _, err := (Adapter{Hook: true}).Latest(context.Background()); err == nil {
		t.Fatal("without network the latest release is an error")
	}
}

func TestAdapterCache(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("POLIX_CACHE_DIR", filepath.Join(dir, "polix"))
	a := Adapter{}
	if c := a.LoadCache(context.Background()); c != (version.Cache{}) {
		t.Fatalf("empty cache: %+v", c)
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := a.SaveCache(context.Background(), version.Cache{CheckedAt: now, Latest: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	if c := a.LoadCache(context.Background()); c.Latest != "0.2.0" || !c.CheckedAt.Equal(now) {
		t.Fatalf("cache: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(dir, "polix", "update.json")); err != nil {
		t.Fatalf("the cache is update.json in POLIX_CACHE_DIR: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "polix", "update.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if c := a.LoadCache(context.Background()); c != (version.Cache{}) {
		t.Fatalf("a broken cache is empty: %+v", c)
	}
}

// The workflow of the repository of the working folder is rejected as polix.yaml says; outside a repository or
// without polix.yaml nothing is.
func TestAdapterWorkflowRejected(t *testing.T) {
	ctx := context.Background()
	outside := t.TempDir()
	if err := (Adapter{Dir: outside}).WorkflowRejected(ctx, "0.1.0"); err != nil {
		t.Fatalf("outside a repository: %v", err)
	}
	repo := t.TempDir()
	if out, err := exec.Command("git", "-C", repo, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	a := Adapter{Dir: sub}
	if err := a.WorkflowRejected(ctx, "0.1.0"); err != nil {
		t.Fatalf("without polix.yaml: %v", err)
	}
	for src, ahead := range map[string]bool{
		"version: 1\nrequires: 0.2.0\n": true,
		"version: 1\nan_option: true\n": true,
		"version: 1\non:\n  pre_edit:\n    - action: { block: x }\n      an_option: 1\n": true,
		"version: 2\n":                      true,
		"version: 1\nmax_stop_blocks: -1\n": false,
		"version: 1\n":                      false,
	} {
		if err := os.WriteFile(filepath.Join(repo, "polix.yaml"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		err := a.WorkflowRejected(ctx, "0.1.0")
		if got := err != nil && version.NeedsNewer(err.Error()); got != ahead {
			t.Errorf("polix.yaml %q: rejected %v, ahead %v, want %v", src, err, got, ahead)
		}
	}
}

// Refresh and Doctor run the binary from the working folder with the arguments of the commands the update runs
// on the new binary, which every release must keep (see docs/polix.md).
func TestAdapterRunsTheBinary(t *testing.T) {
	t.Setenv(fakeEnv, "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "work")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := Adapter{Dir: dir}
	ctx := context.Background()
	stdout, stderr, err := a.Refresh(ctx, exe, true)
	if err != nil || stdout != "ran __refresh --quiet in work\n" || stderr != "to stderr\n" {
		t.Fatalf("Refresh of the hook: %q %q %v", stdout, stderr, err)
	}
	stdout, _, err = a.Refresh(ctx, exe, false)
	if err != nil || stdout != "ran install --user --yes --quiet in work\nran __refresh --hooks in work\n" {
		t.Fatalf("Refresh of a person: %q %v", stdout, err)
	}
	stdout, _, err = a.Doctor(ctx, exe)
	if err != nil || stdout != "ran doctor --hook in work\n" {
		t.Fatalf("Doctor: %q %v", stdout, err)
	}
	t.Setenv(fakeFail, "1")
	if stdout, _, err = a.Refresh(ctx, exe, false); err == nil || strings.Contains(stdout, "__refresh") {
		t.Fatalf("a failed install stops the refresh: %q %v", stdout, err)
	}
}

func TestAdapterExecutable(t *testing.T) {
	exe, err := Adapter{}.Executable(context.Background())
	if err != nil || !filepath.IsAbs(exe) {
		t.Fatalf("Executable = %q, %v", exe, err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("the running binary exists: %v", err)
	}
}
