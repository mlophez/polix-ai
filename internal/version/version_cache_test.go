package version

import (
	"errors"
	"testing"
	"time"

	"github.com/mlophez/polix-ai/internal/workflow"
)

var cacheNow = time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

func TestParseCache(t *testing.T) {
	c := Cache{CheckedAt: cacheNow, Latest: "0.2.0"}
	data, err := c.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"checked_at":"2026-11-05T12:00:00Z","latest":"0.2.0"}` {
		t.Fatalf("the file keeps its shape, without a failure: %s", data)
	}
	if got := ParseCache(data); !got.CheckedAt.Equal(cacheNow) || got.Latest != "0.2.0" || !got.FailedAt.IsZero() {
		t.Fatalf("round trip: %+v", got)
	}
	failed, _ := c.Fail(cacheNow).Encode()
	if got := ParseCache(failed); !got.FailedAt.Equal(cacheNow) {
		t.Fatalf("the failure is kept: %s %+v", failed, got)
	}
	for _, bad := range []string{"", "{", `{"latest":"nightly"}`} {
		if got := ParseCache([]byte(bad)); got != (Cache{}) {
			t.Errorf("ParseCache(%q) = %+v, want an empty cache", bad, got)
		}
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
	} {
		if got := NeedsNewer(problems); got != want {
			t.Errorf("NeedsNewer(%q) = %v, want %v", problems, got, want)
		}
	}
	// The problems of a polix.yaml in the old vocabulary, as the workflow reports them, never need a newer polix; a
	// name it does not know among them may.
	old := "version: 1\non:\n  session_start:\n    - context: x\n  turn_stop:\n    - if: { done: x }\n      action: { deny: x }\n    - context: x\n"
	if got := versionCacheProblems(t, old); NeedsNewer(got) {
		t.Errorf("the old vocabulary needs a newer polix:\n%s", got)
	}
	if got := versionCacheProblems(t, old+"    - action: { block: x }\n      an_option: y\n"); !NeedsNewer(got) {
		t.Errorf("an unknown option among the old names:\n%s", got)
	}
}

// versionCacheProblems returns the problems the workflow reports for src, joined as the hooks see them.
func versionCacheProblems(t *testing.T, src string) string {
	t.Helper()
	_, err := workflow.Parse([]byte(src), "dev")
	if _, ok := errors.AsType[workflow.Invalid](err); !ok {
		t.Fatalf("not a workflow with problems: %v", err)
	}
	return err.Error()
}
