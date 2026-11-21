package version

import (
	"errors"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/semver"
)

var cacheNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

// The versions of the cache are text with the semantics of internal/platform/semver: a development build, an empty
// text and 0.0.0 are no release.
func TestReleased(t *testing.T) {
	for v, want := range map[string]bool{"0.2.0": true, "1.0.0-rc1": true, "dev": false, "": false, "0.0.0": false} {
		if got := semver.IsRelease(v); got != want {
			t.Errorf("IsRelease(%q) = %v, want %v", v, got, want)
		}
	}
}

// A missing number counts as 0 and a text that is no version as 0.0.0, as the cache compares its versions.
func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.2.0", "v0.2.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"dev", "0.0.1", -1},
		{"0.0.1", "nightly", 1},
		{"0.3", "0.2.0", 1},
		{"1.2", "1.2.0", 0},
	}
	for _, c := range cases {
		if got := semver.Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLoadedValid(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name   string
		loaded Loaded
		valid  bool
	}{
		{"no file", Loaded{}, true},
		{"a cache", Loaded{Cache: Cache{Latest: "0.2.0"}, Exists: true}, true},
		{"a file that cannot be read", Loaded{Exists: true, Err: boom}, true},
		{"a cache with the error", Loaded{Cache: Cache{Latest: "0.2.0"}, Exists: true, Err: boom}, false},
		{"a cache without the file", Loaded{Cache: Cache{Latest: "0.2.0"}}, false},
		{"a cache that is not valid", Loaded{Cache: Cache{Latest: "nightly"}, Exists: true}, false},
	} {
		if err := tc.loaded.Valid(); (err == nil) != tc.valid {
			t.Errorf("%s: Valid() = %v, want valid %v", tc.name, err, tc.valid)
		}
	}
}

// Fail empties the cache and keeps why, on a copy.
func TestLoadedFail(t *testing.T) {
	boom := errors.New("boom")
	l := Loaded{Cache: Cache{Latest: "nightly"}, Exists: true}
	failed := l.Fail(boom)
	if failed.Cache != (Cache{}) || !failed.Exists || !errors.Is(failed.Err, boom) || failed.Valid() != nil {
		t.Fatalf("Fail = %+v", failed)
	}
	if l.Cache.Latest != "nightly" || l.Err != nil {
		t.Fatal("Fail returns a copy")
	}
}

func TestCacheValid(t *testing.T) {
	if err := (Cache{}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Cache{Latest: "0.2.0"}).Valid(); err != nil {
		t.Fatal(err)
	}
	if err := (Cache{Latest: "dev"}).Valid(); err == nil {
		t.Fatal("a latest release that is not a version")
	}
}

// The time of a check is only saved once this binary is the latest; a check that leaves it behind is repeated.
func TestCacheSaw(t *testing.T) {
	old := Cache{CheckedAt: cacheNow.Add(-2 * time.Hour), Latest: "0.1.0", FailedAt: cacheNow.Add(-time.Hour)}
	behind := old.Saw(cacheNow, "0.1.0", "0.2.0")
	if behind.Latest != "0.2.0" || !behind.CheckedAt.Equal(old.CheckedAt) || !behind.FailedAt.Equal(old.FailedAt) {
		t.Fatalf("behind: %+v", behind)
	}
	if dev := old.Saw(cacheNow, "dev", "0.2.0"); !dev.CheckedAt.Equal(old.CheckedAt) {
		t.Fatalf("a development build is always behind: %+v", dev)
	}
	latest := old.Saw(cacheNow, "0.2.0", "0.2.0")
	if latest != (Cache{CheckedAt: cacheNow, Latest: "0.2.0"}) {
		t.Fatalf("the latest forgets the failure: %+v", latest)
	}
	if old.Latest != "0.1.0" {
		t.Fatal("Saw returns a copy")
	}
}

func TestCacheShouldCheck(t *testing.T) {
	ahead := errors.New("polix.yaml requires polix 0.3.0 or newer and this is 0.1.0: run polix update")
	plain := errors.New("polix.yaml: max_stop_blocks must not be negative")
	cases := []struct {
		name     string
		c        Cache
		running  string
		rejected error
		want     bool
	}{
		{"never checked", Cache{}, "0.1.0", nil, true},
		{"a development build never asks", Cache{}, "dev", nil, false},
		{"checked within the hour", Cache{CheckedAt: cacheNow.Add(-time.Minute)}, "0.1.0", nil, false},
		{"checked an hour ago", Cache{CheckedAt: cacheNow.Add(-CheckEvery)}, "0.1.0", nil, true},
		{"a check in the future", Cache{CheckedAt: cacheNow.Add(time.Hour)}, "0.1.0", nil, true},
		{"a newer release known", Cache{CheckedAt: cacheNow, Latest: "0.2.0"}, "0.1.0", nil, true},
		{"the latest known", Cache{CheckedAt: cacheNow, Latest: "0.1.0"}, "0.1.0", nil, false},
		{"a workflow ahead", Cache{CheckedAt: cacheNow, Latest: "0.1.0"}, "0.1.0", ahead, true},
		{"a plain mistake of the workflow", Cache{CheckedAt: cacheNow}, "0.1.0", plain, false},
		{"within the wait after a failure", Cache{FailedAt: cacheNow.Add(-time.Minute)}, "0.1.0", ahead, false},
		{"after the wait", Cache{FailedAt: cacheNow.Add(-RetryAfter - time.Minute)}, "0.1.0", nil, true},
	}
	for _, c := range cases {
		if got := c.c.ShouldCheck(cacheNow, c.running, c.rejected); got != c.want {
			t.Errorf("%s: ShouldCheck = %v, want %v", c.name, got, c.want)
		}
	}
}

// A nested option, or a later version of the schema, may come from a newer polix too; a plain mistake may not, nor
// a name of the old vocabulary, which comes with its new spelling.
func TestNeedsNewer(t *testing.T) {
	for problems, want := range map[string]bool{
		"polix.yaml requires polix 0.2.0 or newer and this is 0.1.0: run polix update": true,
		`polix.yaml:2: unknown key "an_option"`:                                        true,
		"polix.yaml: version must be 1, got 2":                                         true,
		"polix.yaml: version must be 1, got 12":                                        true,
		"polix.yaml: version must be 1, got 0":                                         false,
		"polix.yaml: max_stop_blocks must be positive, got -1":                         false,
		// CUJ-04: the names of on that git can never give its hooks, in check_on_save.
		"polix.yaml:4: guardrails.check_on_save: unknown placeholder {path} (githooks allow {branch}, or a variable of vars)": false,
		`polix.yaml:4: unknown key "wait_agents" in guardrails.check_on_save (run, changed, message)`:                         false,
		// A measure git can compute may come from a newer polix, in check_on_save or in githooks, and so may every
		// placeholder of the entries written in githooks.
		"polix.yaml:4: guardrails.check_on_save: unknown placeholder {changed_lines} (githooks allow {branch}, or a variable of vars)": true,
		"polix.yaml:4: githooks.pre_commit: unknown placeholder {changed_files} (githooks allow {branch}, or a variable of vars)":      true,
		"polix.yaml:4: githooks.pre_commit: unknown placeholder {path} (githooks allow {branch}, or a variable of vars)":               true,
	} {
		if got := Problems(problems).NeedsNewer(); got != want {
			t.Errorf("Problems(%q).NeedsNewer() = %v, want %v", problems, got, want)
		}
	}
	// The problems of a polix.yaml in the old vocabulary, as the workflow reports them, never need a newer polix; a
	// name it does not know among them may.
	// version: 1
	// on:
	//   session_start:
	//     - context: x
	//   turn_stop:
	//     - if: { done: x }
	//       action: { deny: x }
	//     - context: x
	old := []workflow.Entry{
		{Line: 6, Keys: []string{"if", "action"}, OldIf: workflow.Cond{"done": "x"},
			Action: workflow.Action{Keys: []string{"deny"}}},
		{Line: 8, Keys: []string{"context"}},
	}
	if got := versionCacheProblems(t, old); got.NeedsNewer() {
		t.Errorf("the old vocabulary needs a newer polix:\n%s", got)
	}
	//     - action: { block: x }
	//       an_option: y
	option := workflow.Entry{Line: 9, Keys: []string{"action", "an_option"},
		Action: workflow.Action{Block: &workflow.Block{Message: "x"}, Keys: []string{workflow.VerbBlock}}}
	if got := versionCacheProblems(t, append(old, option)); !got.NeedsNewer() {
		t.Errorf("an unknown option among the old names:\n%s", got)
	}
}

// versionCacheProblems returns the problems the workflow reports with the entries of turn_stop, joined as the hooks
// see them.
func versionCacheProblems(t *testing.T, turnStop []workflow.Entry) Problems {
	t.Helper()
	w := workflow.Workflow{Version: 1, EventLines: map[string]int{"session_start": 3, "turn_stop": 5},
		On: map[string][]workflow.Entry{"session_start": {{Line: 4, Keys: []string{"context"}}}, "turn_stop": turnStop}}
	err := w.Valid()
	if _, ok := errors.AsType[workflow.InvalidError](err); !ok {
		t.Fatalf("not a workflow with problems: %v", err)
	}
	return Problems(err.Error())
}

// TestCacheLoaded tests what a file of the cache loads: a valid cache as it is, without the file an empty cache, and
// an empty cache with why for a file that cannot be read or decoded or a cache that is not valid.
func TestCacheLoaded(t *testing.T) {
	// A valid cache is loaded as it is; without the file, an empty cache.
	t.Run("a valid cache", func(t *testing.T) {
		c := Cache{CheckedAt: cacheNow, Latest: "0.2.0", FailedAt: cacheNow}
		if got := c.Loaded(true, nil); got.Cache != c || !got.Exists || got.Err != nil {
			t.Fatalf("got %+v", got)
		}
		if got := (Cache{}).Loaded(false, nil); got != (Loaded{}) {
			t.Fatalf("no file: %+v", got)
		}
	})

	// A file that cannot be read or decoded, or a cache that is not valid, is an empty cache with why.
	t.Run("empties a broken cache", func(t *testing.T) {
		boom := errors.New("unexpected end of JSON input")
		if got := (Cache{}).Loaded(true, boom); got.Cache != (Cache{}) || !errors.Is(got.Err, boom) || !got.Exists {
			t.Fatalf("a broken file: %+v", got)
		}
		if got := (Cache{Latest: "nightly"}).Loaded(true, nil); got.Cache != (Cache{}) || got.Err == nil {
			t.Fatalf("a latest release that is not a version: %+v", got)
		}
	})
}
