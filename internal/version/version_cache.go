package version

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/mlophez/polix-ai/internal/workflow"
)

// Cache remembers the last check of the latest release, so the session start hook asks at most once an hour, and
// the last update that failed, so a failure that repeats (a folder it cannot write, a proxy that cuts the
// download) is retried after a while instead of at every session start.
type Cache struct {
	CheckedAt time.Time // when this binary was last seen to be the latest release; zero when never
	Latest    string    // the latest release known
	FailedAt  time.Time // when the last update of the session start hook failed; zero when none did
}

// cacheJSON is the shape of the file of the cache.
type cacheJSON struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	FailedAt  time.Time `json:"failed_at,omitzero"`
}

const (
	// CheckEvery spaces the checks of the session start hook.
	CheckEvery = time.Hour
	// RetryAfter spaces the attempts of the session start hook after an update that failed.
	RetryAfter = 15 * time.Minute
)

// newerSchema matches the problem of a workflow of a later version of the schema.
var newerSchema = regexp.MustCompile(`version must be 1, got ([2-9]|\d{2,})`)

// ParseCache decodes the file of the cache; a file that cannot be decoded is an empty cache, which only means
// asking again.
func ParseCache(data []byte) Cache {
	var j cacheJSON
	if err := json.Unmarshal(data, &j); err != nil {
		return Cache{}
	}
	c := Cache(j)
	if err := c.Valid(); err != nil {
		return Cache{}
	}
	return c
}

// Valid tells why c is not a cache: a latest release that is not a version.
func (c Cache) Valid() error {
	if c.Latest == "" {
		return nil
	}
	_, err := Parse(c.Latest)
	return err
}

// Encode writes the cache in the shape of its file.
func (c Cache) Encode() ([]byte, error) {
	return json.Marshal(cacheJSON(c))
}

// Saw records that latest is the latest release, asked at now by the polix of version running. The time of the
// check is only recorded, and an earlier failure forgotten, when running is that release or a later one: a check
// that leaves this binary behind (a failed download, --check) is repeated at the next session start.
func (c Cache) Saw(now time.Time, running, latest string) Cache {
	saw := c
	saw.Latest = latest
	if Released(running) && Compare(running, latest) >= 0 {
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
	case !Released(running), within(now, c.FailedAt, RetryAfter):
		return false
	case !within(now, c.CheckedAt, CheckEvery):
		return true
	case Released(c.Latest) && Compare(running, c.Latest) < 0:
		return true
	}
	return rejected != nil && NeedsNewer(rejected.Error())
}

// NeedsNewer reports whether the problems of a workflow may come from a newer polix than the running one: it
// requires one, it names something this one does not know (not a name of the old vocabulary, which comes with its
// new spelling), or it has a later version of the schema.
func NeedsNewer(problems string) bool {
	if strings.Contains(problems, "requires polix") || newerSchema.MatchString(problems) {
		return true
	}
	for line := range strings.SplitSeq(problems, "\n") {
		if strings.Contains(line, "unknown") && !workflow.Renamed(line) {
			return true
		}
	}
	return false
}

// within reports whether t is less than d before now. A time after now (saved with a clock that was ahead) is not
// within: it must not stop the checks until the clock reaches it.
func within(now, t time.Time, d time.Duration) bool {
	since := now.Sub(t)
	return since >= 0 && since < d
}
