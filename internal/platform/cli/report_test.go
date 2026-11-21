package cli

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

var escape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// doctorReport is the report of CUJ-01.
func doctorReport() Report {
	return Report{Title: "polix doctor · polix dev", Sections: []Section{
		{Title: "Machine", Rows: []Row{
			{Level: LevelWarn, Area: "version", Message: "polix dev is a development build",
				Fix: "install a release with polix update --version <version>"},
			{Level: LevelOK, Area: "path", Message: "the hooks run /usr/local/bin/polix"}}},
		{Title: "Repository", Detail: "/work/repo", Rows: []Row{
			{Level: LevelOK, Area: "git", Message: "repository at /work/repo"},
			{Level: LevelOK, Area: "requirements", Message: "3 programs of requirements on the PATH"}}},
		{Title: "Empty"},
		{Title: "Agents", Rows: []Row{
			{Level: LevelOK, Area: "hooks", Message: "registered in /work/repo/.claude/settings.json"},
			{Level: LevelOK, Area: "plugin", Message: "polix@polix 0.0.0-dev"}}},
	}}
}

// AC-01: plain output has the level words, the areas padded to the longest, the fix under the message, no empty
// section, the counts and no escape.
func TestReportPlain(t *testing.T) {
	var b bytes.Buffer
	NewUI(&b).Report(doctorReport())
	want := "polix doctor · polix dev\n\n" +
		"Machine\n" +
		"  warn  version       polix dev is a development build\n" +
		"                      → install a release with polix update --version <version>\n" +
		"  ok    path          the hooks run /usr/local/bin/polix\n\n" +
		"Repository  /work/repo\n" +
		"  ok    git           repository at /work/repo\n" +
		"  ok    requirements  3 programs of requirements on the PATH\n\n" +
		"Agents\n" +
		"  ok    hooks         registered in /work/repo/.claude/settings.json\n" +
		"  ok    plugin        polix@polix 0.0.0-dev\n\n" +
		"5 ok · 1 warn · 0 fail\n"
	if b.String() != want {
		t.Fatalf("got %q\nwant %q", b.String(), want)
	}
	if strings.Contains(b.String(), "\x1b") || strings.Contains(b.String(), "Empty") {
		t.Fatal("no escape and no empty section")
	}
}

// AC-02: stripped of its escapes, the rich output is LIT-02, and each part carries the escape of its style.
func TestReportRich(t *testing.T) {
	var b bytes.Buffer
	u := &UI{w: &b, rich: true}
	u.Report(doctorReport())
	want := "polix doctor · polix dev\n\n" +
		"Machine\n" +
		"  !  version       polix dev is a development build\n" +
		"                   → install a release with polix update --version <version>\n" +
		"  ✓  path          the hooks run /usr/local/bin/polix\n\n" +
		"Repository  /work/repo\n" +
		"  ✓  git           repository at /work/repo\n" +
		"  ✓  requirements  3 programs of requirements on the PATH\n\n" +
		"Agents\n" +
		"  ✓  hooks         registered in /work/repo/.claude/settings.json\n" +
		"  ✓  plugin        polix@polix 0.0.0-dev\n\n" +
		"5 ok · 1 warn · 0 fail\n"
	if got := escape.ReplaceAllString(b.String(), ""); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	for _, part := range []string{
		styleBold.Render("polix doctor · polix dev"), styleBold.Render("Machine"), styleBold.Render("Repository"),
		styleMuted.Render("/work/repo"), styleMuted.Render("requirements"), styleOK.Render("✓"), styleWarn.Render("!"),
		styleCode.Render("→ install a release with polix update --version <version>"),
		styleOK.Render("5 ok"), styleWarn.Render("1 warn"), styleFail.Render("0 fail"), styleMuted.Render(" · "),
	} {
		if !strings.Contains(b.String(), part) {
			t.Errorf("missing %q", part)
		}
	}
	var f bytes.Buffer
	(&UI{w: &f, rich: true}).Report(Report{Sections: []Section{{Title: "S", Rows: []Row{{Level: LevelFail, Message: "m"}}}}})
	if !strings.Contains(f.String(), styleFail.Render("✗")) {
		t.Fatalf("the symbol of fail: %q", f.String())
	}
}

// Without areas and without title there is no area column and no header.
func TestReportWithoutAreas(t *testing.T) {
	var b bytes.Buffer
	NewUI(&b).Report(Report{Sections: []Section{
		{Title: "Workflow", Detail: "polix.yaml", Rows: []Row{
			{Level: LevelFail, Message: "polix.yaml:3: unknown key foo", Fix: "run polix update"}}},
		{Title: "Rules", Detail: "ai/rules/", Rows: []Row{{Level: LevelFail, Message: "go.yaml GO-9: bad"}}},
	}})
	want := "Workflow  polix.yaml\n" +
		"  fail  polix.yaml:3: unknown key foo\n" +
		"        → run polix update\n\n" +
		"Rules  ai/rules/\n" +
		"  fail  go.yaml GO-9: bad\n\n" +
		"0 ok · 0 warn · 2 fail\n"
	if b.String() != want {
		t.Fatalf("got %q\nwant %q", b.String(), want)
	}
}

// NewUI is rich from a 16-colour profile up and plain below it: CLICOLOR_FORCE gives colors to a writer that is no
// terminal, NO_COLOR takes them away.
func TestNewUIProfile(t *testing.T) {
	cases := []struct {
		name  string
		env   map[string]string
		rich  bool
		want  string
		avoid string
		level colorprofile.Profile // the profile the case must land on, to stay on the boundary; zero skips the check
	}{
		{"ansi", map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "", "COLORTERM": "", "TERM": "xterm"}, true, "✓", "  ok  ", 0},
		{"none", map[string]string{"CLICOLOR_FORCE": "", "NO_COLOR": "1", "COLORTERM": "", "TERM": "xterm"}, false, "  ok  ", "✓", 0},
		{"ascii", map[string]string{"TTY_FORCE": "1", "CLICOLOR_FORCE": "", "NO_COLOR": "1", "COLORTERM": "", "TERM": "xterm"}, false, "  ok  ", "✓", colorprofile.ASCII},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			var b bytes.Buffer
			if c.level != 0 {
				if got := colorprofile.NewWriter(&b, os.Environ()).Profile; got != c.level {
					t.Fatalf("profile = %v, want %v", got, c.level)
				}
			}
			u := NewUI(&b)
			if u.rich != c.rich {
				t.Fatalf("rich = %v, want %v", u.rich, c.rich)
			}
			u.Report(Report{Sections: []Section{{Title: "S", Rows: []Row{{Level: LevelOK, Message: "m"}}}}})
			out := b.String()
			if !strings.Contains(out, c.want) || strings.Contains(out, c.avoid) {
				t.Fatalf("output %q: want %q, not %q", out, c.want, c.avoid)
			}
		})
	}
}
