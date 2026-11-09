package version

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/app/workflow"
	"github.com/mlophez/polix-ai/internal/platform/semver"
)

// Cache remembers the last check of the latest release, so the session start hook asks at most once an hour, and
// the last update that failed, so a failure that repeats (a folder it cannot write, a proxy that cuts the
// download) is retried after a while instead of at every session start. Its versions are text, compared with the
// semantics of internal/platform/semver: a development build ("dev") is not a release, and a missing number counts
// as 0.
type Cache struct {
	CheckedAt time.Time // when this binary was last seen to be the latest release; zero when never
	Latest    string    // the latest release known
	FailedAt  time.Time // when the last update of the session start hook failed; zero when none did
}

// Load loads the cache of the file at path. It is the loader of the use case LoadVersion
// (internal/app/version/load), which cmd/polix builds once and passes to every use case that reads the cache. It
// never fails: what stops the reading is data of its result.
type Load func(ctx context.Context, path string) Loaded

// Loaded is what Load read from the file of the cache. A file that cannot be read or decoded, or holds a cache that
// is not valid, is an empty cache with its error: it only means asking again.
type Loaded struct {
	Cache  Cache // the cache of the file; empty without it or with Err
	Exists bool  // whether the file exists
	Err    error // why the file cannot be read, decoded or holds no valid cache; nil when it can or does not exist
}

// Problems are the problems of a workflow as one text, one per line, as the hooks and polix config check see them.
type Problems string

const (
	// CheckEvery spaces the checks of the session start hook.
	CheckEvery = time.Hour
	// RetryAfter spaces the attempts of the session start hook after an update that failed.
	RetryAfter = 15 * time.Minute
)

// newerSchema matches the problem of a workflow of a later version of the schema.
var newerSchema = regexp.MustCompile(`version must be 1, got ([2-9]|\d{2,})`)

// Valid tells why c is not a cache: a latest release that is not a version like 1.2.3.
func (c Cache) Valid() error {
	if c.Latest != "" && !semver.Valid(c.Latest) {
		return fmt.Errorf("%q is not a version like 1.2.3", c.Latest)
	}
	return nil
}

// Valid tells why l is not what the file of the cache loads: a cache with the error of its reading, a cache without
// the file, or a cache that is not valid.
func (l Loaded) Valid() error {
	switch {
	case l.Err != nil && l.Cache != (Cache{}):
		return errors.New("a cache loaded from a file that cannot be read")
	case !l.Exists && l.Cache != (Cache{}):
		return errors.New("a cache loaded without its file")
	}
	return l.Cache.Valid()
}

// Fail returns l failed with err: an empty cache, which only means asking again, and why.
func (l Loaded) Fail(err error) Loaded {
	failed := l
	failed.Cache, failed.Err = Cache{}, err
	return failed
}

// Saw records that latest is the latest release, asked at now by the polix of version running. The time of the
// check is only recorded, and an earlier failure forgotten, when running is that release or a later one: a check
// that leaves this binary behind (a failed download, --check) is repeated at the next session start.
func (c Cache) Saw(now time.Time, running, latest string) Cache {
	saw := c
	saw.Latest = latest
	if semver.IsRelease(running) && semver.Compare(running, latest) >= 0 {
		saw.CheckedAt, saw.FailedAt = now, time.Time{}
	}
	return saw
}

// Fail records that an update of the session start hook failed at now, so it waits RetryAfter before trying again.
func (c Cache) Fail(now time.Time) Cache {
	failed := c
	failed.FailedAt = now
	return failed
}

// ShouldCheck decides whether the session start hook of the polix of version running asks for the latest release
// at now. A release build asks once every CheckEvery, at once when the cache already knows a newer release (a
// polix doctor or --check saw it) or when rejected, the error of this binary on the workflow of the repository,
// may come from a newer polix; and never within RetryAfter of an update that failed. A development build never
// asks.
func (c Cache) ShouldCheck(now time.Time, running string, rejected error) bool {
	switch {
	case !semver.IsRelease(running), c.retrying(now):
		return false
	case !c.checked(now):
		return true
	case semver.IsRelease(c.Latest) && semver.Compare(running, c.Latest) < 0:
		return true
	}
	return rejected != nil && Problems(rejected.Error()).NeedsNewer()
}

// retrying reports whether now is less than RetryAfter after the last update that failed. A time after now (saved
// with a clock that was ahead) is not: it must not stop the checks until the clock reaches it.
func (c Cache) retrying(now time.Time) bool {
	since := now.Sub(c.FailedAt)
	return since >= 0 && since < RetryAfter
}

// checked reports whether now is less than CheckEvery after the last check, with the clock ahead as in retrying.
func (c Cache) checked(now time.Time) bool {
	since := now.Sub(c.CheckedAt)
	return since >= 0 && since < CheckEvery
}

// NeedsNewer reports whether the problems may come from a newer polix than the running one: they say it requires
// one, they name something this one does not know (not a name of the old vocabulary, which comes with its new
// spelling), or they have a later version of the schema.
func (p Problems) NeedsNewer() bool {
	problems := string(p)
	if strings.Contains(problems, "requires polix") || newerSchema.MatchString(problems) {
		return true
	}
	for line := range strings.SplitSeq(problems, "\n") {
		if strings.Contains(line, "unknown") && !workflow.Problem(line).Renamed() {
			return true
		}
	}
	return false
}
