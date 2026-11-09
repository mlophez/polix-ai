package github

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

// releases serves a fake releases page with one release, v0.2.0, whose asset is bin.
func releases(t *testing.T, asset string, bin []byte, sum string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/latest", func(w http.ResponseWriter, r *http.Request) {
		if ua := r.UserAgent(); ua != "polix-update" {
			http.Error(w, "user agent "+ua, http.StatusForbidden)
			return
		}
		http.Redirect(w, r, "/tag/v0.2.0", http.StatusFound)
	})
	mux.HandleFunc("/download/v0.2.0/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("0000  polix_other_os\n" + sum + "  " + asset + "\n"))
	})
	// The asset is served through a redirect, as GitHub does.
	mux.HandleFunc("/download/v0.2.0/"+asset, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/assets/"+asset, http.StatusFound)
	})
	mux.HandleFunc("/assets/"+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bin) })
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
	asset := AssetName("polix", runtime.GOOS, runtime.GOARCH)
	srv := releases(t, asset, bin, sha(bin))
	c := New(srv.URL+"/", "polix-update")
	latest, err := c.Latest(context.Background())
	if err != nil || latest != "0.2.0" {
		t.Fatalf("Latest = %q, %v", latest, err)
	}
	got, err := c.Download(context.Background(), "v0.2.0", asset)
	if err != nil || string(got) != string(bin) {
		t.Fatalf("Download = %q, %v", got, err)
	}
}

// An asset that does not match checksums.txt is never returned.
func TestDownloadRejectsTamperedBinary(t *testing.T) {
	srv := releases(t, "polix_linux_amd64", []byte("tampered"), sha([]byte("original")))
	if _, err := New(srv.URL, "polix-update").Download(context.Background(), "0.2.0", "polix_linux_amd64"); err == nil ||
		!strings.Contains(err.Error(), "checksum") {
		t.Fatalf("want a checksum error, got %v", err)
	}
}

// An asset without an entry in checksums.txt is never downloaded.
func TestDownloadWithoutChecksum(t *testing.T) {
	srv := releases(t, "polix_linux_amd64", []byte("bin"), sha([]byte("bin")))
	if _, err := New(srv.URL, "polix-update").Download(context.Background(), "0.2.0", "polix_plan9_arm"); err == nil ||
		!strings.Contains(err.Error(), "no entry for polix_plan9_arm") {
		t.Fatalf("want a missing entry, got %v", err)
	}
}

// While a repository has no release, GitHub sends releases/latest back to the releases page.
func TestLatestWithoutReleases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases", http.StatusFound)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "polix-update").Latest(context.Background()); err == nil || !strings.Contains(err.Error(), "no release") {
		t.Fatalf("want no release, got %v", err)
	}
}

func TestAssetName(t *testing.T) {
	if got := AssetName("polix", "linux", "arm64"); got != "polix_linux_arm64" {
		t.Fatalf("AssetName = %q", got)
	}
	if got := AssetName("polix", "windows", "amd64"); got != "polix_windows_amd64.exe" {
		t.Fatalf("AssetName = %q", got)
	}
}

func TestReplace(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "polix")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Replace(exe, ".polix-update-", []byte("new")); err != nil {
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
	Cleanup(exe)
	if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
		t.Fatalf("Cleanup leaves only the binary: %v", entries)
	}
}

// On Windows the old binary is moved aside; when the new one cannot take its place, the old one is put back, so
// the machine is never left without the binary.
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
	if err := Replace(exe, ".polix-update-", []byte("new")); err == nil {
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
	if err := Replace(exe, ".polix-update-", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(exe); string(data) != "new" {
		t.Fatalf("moved aside and replaced: %q", data)
	}
}

func TestWritable(t *testing.T) {
	dir := t.TempDir()
	if err := Writable(filepath.Join(dir, "polix"), ".polix-update-"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("the probe is removed: %v", entries)
	}
	if err := Writable(filepath.Join(dir, "missing", "polix"), ".polix-update-"); err == nil {
		t.Fatal("a folder that cannot be written is reported")
	}
}

func TestExecutable(t *testing.T) {
	exe, err := Executable()
	if err != nil || !filepath.IsAbs(exe) {
		t.Fatalf("Executable = %q, %v", exe, err)
	}
}
