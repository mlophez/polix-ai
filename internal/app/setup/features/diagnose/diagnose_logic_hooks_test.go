package diagnose

import (
	"strings"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// A matcher of PreToolUse without Skill, as an earlier polix wrote it, is a warning with its fix; the other missing
// tools stay a failure apart, and the matcher of this release gives no finding.
func TestHooksSkillMatcher(t *testing.T) {
	const skill = "the matcher of PreToolUse does not cover Skill"
	const old = `"matcher": "Edit|Write|MultiEdit|NotebookEdit|Bash`
	settings := string(exampleSettings(t))
	pre := strings.Replace(settings, old+`|Skill"`, old+`"`, 1)
	if pre == settings {
		t.Fatal("the example holds the matcher with Skill")
	}
	cases := []struct {
		name, settings string
		warn, fail     string
	}{
		{"old matcher", pre, skill, ""},
		{"edit", strings.Replace(settings, `"matcher": "Edit|Write|MultiEdit|NotebookEdit|Bash|Skill"`, `"matcher": "Edit"`, 1), skill,
			"the matcher of PreToolUse does not cover Write, MultiEdit, NotebookEdit, Bash"},
		{"current matcher", settings, "", ""},
	}
	for _, c := range cases {
		i := healthy(t)
		i.ClaudeHooks[i.Setup.HooksFile(setup.Claude)] = hooksOf([]byte(c.settings))
		out := Decide(i, false)
		var warn, fail, other bool
		for _, f := range out.Findings {
			if f.Area != "hooks" || !strings.Contains(f.Text(), "the matcher of PreToolUse") {
				continue
			}
			switch {
			case strings.Contains(f.Message, "Skill") && f.Level == setup.LevelWarn && f.Fix == "run polix install":
				warn = true
			case f.Level == setup.LevelFail && f.Message == c.fail && !strings.Contains(f.Message, "Skill"):
				fail = true
			default:
				other = true
			}
		}
		if warn != (c.warn != "") || fail != (c.fail != "") || other || out.Failed != (c.fail != "") {
			t.Errorf("%s: warn %v fail %v other %v failed %v:\n%s", c.name, warn, fail, other, out.Failed, text(out))
		}
	}
}
