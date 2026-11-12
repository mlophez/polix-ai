package install

import (
	"os"
	"testing"

	"github.com/mlophez/polix-ai/internal/platform/testenv"
)

// TestMain gives the tests a home of their own: the installed binaries run polix install, update and doctor,
// which read and write below it, and must never touch the agents of the machine that runs the tests. They run
// without the variables git exports to its hooks (see testenv.ClearGitHookEnv), so git never reaches the
// repository being committed.
func TestMain(m *testing.M) {
	testenv.ClearGitHookEnv()
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
