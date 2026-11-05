package install

import (
	"os"
	"testing"
)

// TestMain gives the tests a home of their own: the installed binaries run polix install, update and doctor,
// which read and write below it, and must never touch the agents of the machine that runs the tests.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "polix-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	cache, _ := os.MkdirTemp("", "polix-cache-")
	os.Setenv("POLIX_CACHE_DIR", cache)
	code := m.Run()
	os.RemoveAll(home)
	os.RemoveAll(cache)
	os.Exit(code)
}
