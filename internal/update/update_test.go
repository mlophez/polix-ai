package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// releases serves a fake releases page with one release, v0.2.0, whose binary is bin.
func releases(t *testing.T, bin []byte, sum string) *httptest.Server {
	t.Helper()
	asset := AssetName(runtime.GOOS, runtime.GOARCH)
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/tag/v0.2.0", http.StatusFound)
	})
	mux.HandleFunc("/download/v0.2.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("0000  polix_other_os\n" + sum + "  " + asset + "\n"))
	})
	// The binary is served through a redirect, as GitHub does.
	mux.HandleFunc("/download/v0.2.0/"+asset, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/"+asset, http.StatusFound)
	})
	mux.HandleFunc("/assets/"+asset, func(w http.ResponseWriter, r *http.Request) { w.Write(bin) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestLatestAndDownload(t *testing.T) {
	bin := []byte("new binary")
	srv := releases(t, bin, sha(bin))
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	c := New()
	latest, err := c.Latest(context.Background())
	if err != nil || latest != "0.2.0" {
		t.Fatalf("Latest = %q, %v", latest, err)
	}
	got, err := c.Download(context.Background(), "v0.2.0")
	if err != nil || string(got) != string(bin) {
		t.Fatalf("Download = %q, %v", got, err)
	}
}

// A binary that does not match checksums.txt is never returned.
func TestDownloadRejectsTamperedBinary(t *testing.T) {
	srv := releases(t, []byte("tampered"), sha([]byte("original")))
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	if _, err := New().Download(context.Background(), "0.2.0"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want a checksum error, got %v", err)
	}
}

// While a repository has no release, GitHub sends releases/latest back to the releases page.
func TestLatestWithoutReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	}))
	defer srv.Close()
	t.Setenv("POLIX_RELEASES_URL", srv.URL)
	if _, err := New().Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no release") {
		t.Fatalf("want no release, got %v", err)
	}
}

func TestReplace(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(exe)
	info, _ := os.Stat(exe)
	if string(data) != "new" || (runtime.GOOS != "windows" && info.Mode()&0o111 == 0) {
		t.Fatalf("replaced binary: %q %v", data, info.Mode())
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) > 2 { // the binary, and on Windows the old one
		t.Fatalf("temporary files left: %v", entries)
	}
}

func TestCache(t *testing.T) {
	t.Setenv("POLIX_CACHE_DIR", t.TempDir())
	if c := LoadCache(); !c.CheckedAt.IsZero() {
		t.Fatalf("empty cache: %+v", c)
	}
	if err := SaveCache(Cache{Latest: "0.2.0"}); err != nil {
		t.Fatal(err)
	}
	if c := LoadCache(); c.Latest != "0.2.0" {
		t.Fatalf("cache: %+v", c)
	}
}

// On Windows the old binary is moved aside; when the new one cannot take its place, the old one is put back, so
// the machine is never left without polix.
func TestReplaceRollsBack(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldRename, oldAside := rename, moveAside
	t.Cleanup(func() { rename, moveAside = oldRename, oldAside })
	moveAside = true
	rename = func(from, to string) error {
		if strings.Contains(filepath.Base(from), ".polix-update-") {
			return os.ErrPermission // an antivirus holds the new file
		}
		return os.Rename(from, to)
	}
	if err := Replace(exe, []byte("new")); err == nil {
		t.Fatal("the failed rename is reported")
	}
	if data, err := os.ReadFile(exe); err != nil || string(data) != "old" {
		t.Fatalf("the old binary is back: %q %v", data, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Fatalf("nothing else is left: %v", entries)
	}

	rename = os.Rename
	if err := Replace(exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exe); string(data) != "new" {
		t.Fatalf("moved aside and replaced: %q", data)
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	if err := Writable(filepath.Join(dir, "polix")); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the probe is removed: %v", entries)
	}
	if err := Writable(filepath.Join(dir, "missing", "polix")); err == nil {
		t.Fatal("a folder that cannot be written is reported")
	}
}
