package diagnose

import (
	"testing"

	"github.com/mlophez/polix-ai/internal/app/version"
)

func TestVersionTextParse(t *testing.T) {
	for s, want := range map[versionText]version.Version{
		"0.2.0":      {Minor: 2},
		"v1.2.3":     {Major: 1, Minor: 2, Patch: 3},
		"1.0.0-rc.1": {Major: 1, Pre: "rc.1"},
	} {
		if got, err := s.parse(); err != nil || got != want {
			t.Errorf("versionText(%q).parse() = %+v, %v; want %+v", s, got, err, want)
		}
	}
	for _, s := range []versionText{"0.2", "dev", "", "1.2.3.4", "1.2.3-", "x1.2.3"} {
		if got, err := s.parse(); err == nil || got != (version.Version{}) {
			t.Errorf("versionText(%q).parse() = %+v, %v; want an error and the zero version", s, got, err)
		}
	}
}
