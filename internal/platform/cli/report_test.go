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

// wrapRow is the row of CUJ-03: one long message in one line.
func wrapRow(fix string) Report {
	msg := strings.Join(strings.Fields(`polix@polix 0.0.0-dev: skills
		/polix:add-rule /polix:audit /polix:build, agents polix:auditor polix:builder`), " ")
	return Report{Sections: []Section{{Title: "Agents", Rows: []Row{
		{Level: LevelOK, Area: "plugin", Message: msg, Fix: fix}}}}}
}

// AC-03: a rich ui of a known width wraps the message at word boundaries under its column, and the fix under its text.
func TestReportWrap(t *testing.T) {
	var b bytes.Buffer
	(&UI{w: &b, rich: true, width: 50}).Report(wrapRow(""))
	want := "  ✓  plugin  polix@polix 0.0.0-dev: skills\n" +
		"             /polix:add-rule /polix:audit\n" +
		"             /polix:build, agents polix:auditor\n" +
		"             polix:builder\n"
	got := escape.ReplaceAllString(b.String(), "")
	if !strings.Contains(got, want) {
		t.Fatalf("got %q\nwant it to contain %q", got, want)
	}
	if strings.Join(strings.Fields(got), " ") == "" || !strings.Contains(strings.Join(strings.Fields(got), " "), strings.Join(strings.Fields(want), " ")) {
		t.Fatal("a word was lost")
	}

	b.Reset()
	(&UI{w: &b, rich: true, width: 44}).Report(wrapRow("run polix install --skills and then polix doctor again"))
	got = escape.ReplaceAllString(b.String(), "")
	wantFix := "             → run polix install --skills\n" +
		"               and then polix doctor again\n"
	if !strings.Contains(got, wantFix) {
		t.Fatalf("got %q\nwant it to contain %q", got, wantFix)
	}
	if !strings.Contains(b.String(), styleCode.Render("→ run polix install --skills")) ||
		!strings.Contains(b.String(), styleCode.Render("  and then polix doctor again")) {
		t.Fatalf("each line of the fix carries its style: %q", b.String())
	}
	for line := range strings.SplitSeq(got, "\n") {
		if n := len([]rune(line)); n > 44 {
			t.Errorf("line of %d columns: %q", n, line)
		}
	}

	b.Reset()
	(&UI{w: &b, rich: true, width: 20}).Report(Report{Sections: []Section{{Title: "S", Rows: []Row{
		{Level: LevelOK, Message: "abcdefghijklmnopqrstuvwxyz"}}}}})
	if got := escape.ReplaceAllString(b.String(), ""); !strings.Contains(got, "  ✓  abcdefghijklmno\n     pqrstuvwxyz\n") {
		t.Fatalf("a word longer than the room breaks at it: %q", got)
	}
}

// AC-03: plain output, an unknown width and a room of zero never wrap.
func TestReportWrapPlain(t *testing.T) {
	msg := wrapRow("").Sections[0].Rows[0].Message
	for name, c := range map[string]struct {
		rich  bool
		width int
	}{
		"plain":      {false, 50},
		"no width":   {true, 0},
		"no room":    {true, 8},
		"exact zero": {true, 13},
	} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			(&UI{w: &b, rich: c.rich, width: c.width}).Report(wrapRow(""))
			got := escape.ReplaceAllString(b.String(), "")
			if !strings.Contains(got, msg+"\n") {
				t.Fatalf("the message stays on one line: %q", got)
			}
		})
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

func TestWordWrapKeepsSpacing(t *testing.T) {
	tests := []struct {
		name string
		text string
		room int
		want []string
	}{
		{"fits keeps spacing", "a  b\tc ", 20, []string{"a  b\tc "}},
		{"a tab makes a line that looks like it fits pass the room", "ab\tcd ef", 12, []string{"ab\tcd", "ef"}},
		{"keeps line breaks", "yaml:\n  line 1: x", 40, []string{"yaml:", "  line 1: x"}},
		{"cuts at single spaces", "aaa bbb ccc", 7, []string{"aaa bbb", "ccc"}},
		{"keeps double spaces when cutting", "run  x  y zzzzzz", 9, []string{"run  x  y", "zzzzzz"}},
		{"cut at a double space", "aaaa  bb", 4, []string{"aaaa", "bb"}},
		{"cut at a double space before a word that fits the line", "aaaa  bbbb", 4, []string{"aaaa", "bbbb"}},
		{"cut at a double space before a long word", "aaaa  bbbbbbbbbb", 4, []string{"aaaa", "bbbb", "bbbb", "bb"}},
		{"cut at a trailing space", "aaaa ", 4, []string{"aaaa"}},
		{"breaks long word", "abcdefgh", 3, []string{"abc", "def", "gh"}},
		{"leading space before a long word", " abcdefgh", 4, []string{" abc", "defg", "h"}},
		{"leading spaces before a word that does not fit", "  abcd", 4, []string{"  ab", "cd"}},
		{"leading spaces wider than the room", "      abcdefgh", 4, []string{"abcd", "efgh"}},
		{"a tab-only word after a cut", "aaa \t bbb", 4, []string{"aaa", "bbb"}},
		{"a tab-led remainder after a cut", "aaaaaaa \tb", 9, []string{"aaaaaaa", "b"}},
		{"a tab-led word that does not fit the line", "aa \tb", 4, []string{"aa", "b"}},
		{"invalid byte at the end", "abcd\xe9", 3, []string{"abc", "d\xe9"}},
		{"invalid byte in the middle", "ab\xe9cdefg", 3, []string{"ab\xe9c", "def", "g"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wordWrap(tt.text, tt.room)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("wordWrap(%q, %d) = %q, want %q", tt.text, tt.room, got, tt.want)
			}
		})
	}
}
