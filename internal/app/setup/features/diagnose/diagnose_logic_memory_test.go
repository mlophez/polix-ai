package diagnose

import (
	"errors"
	"slices"
	"testing"

	"github.com/mlophez/polix-ai/internal/app/setup"
)

// memoryOf returns the findings of the area memory of the report.
func memoryOf(out Output) []setup.Finding {
	var f []setup.Finding
	for _, x := range out.Findings {
		if x.Area == "memory" {
			f = append(f, x)
		}
	}
	return f
}

// withMemory returns the healthy inspection with the settings of the project that register polix setting project
// as their auto memory, the local and user settings that exist setting theirs (by path), and the variable env.
func withMemory(t *testing.T, project string, others map[string]string, env string) setup.Inspection {
	t.Helper()
	i := healthy(t)
	settings := hooksOf(exampleSettings(t))
	settings.AutoMemory = project
	i.ClaudeHooks = map[string]setup.HooksFile{i.Setup.HooksFile(setup.Claude): settings}
	for path, value := range others {
		i.ClaudeHooks[path] = setup.HooksFile{AutoMemory: value}
	}
	i.AutoMemoryEnv = env
	return i
}

// CUJ-08, CUJ-09: one finding of the area memory, decided as Claude Code does: the variable, then the local
// settings, then the project ones, then the user ones, the first that sets it wins; on without any of them.
func TestDecideAutoMemory(t *testing.T) {
	s := healthy(t).Setup
	files := s.ClaudeSettingsFiles()
	local, user := files[1], files[2]
	off := setup.Finding{Level: setup.LevelOK, Area: "memory", Message: "auto memory of Claude Code is off"}
	project := setup.Finding{Level: setup.LevelWarn, Area: "memory",
		Message: "auto memory of Claude Code is on", Fix: "run polix install to turn it off in " + s.HooksFile(setup.Claude)}
	localOn := setup.Finding{Level: setup.LevelWarn, Area: "memory",
		Message: local + " turns auto memory of Claude Code on", Fix: "remove autoMemoryEnabled from it"}
	variable := setup.Finding{Level: setup.LevelWarn, Area: "memory",
		Message: "CLAUDE_CODE_DISABLE_AUTO_MEMORY=0 turns auto memory of Claude Code on", Fix: "unset it"}
	for name, c := range map[string]struct {
		project string
		others  map[string]string
		env     string
		want    setup.Finding
	}{
		"off in the project":              {"false", nil, "", off},
		"project without the key":         {"", nil, "", project},
		"project true":                    {"true", nil, "", project},
		"project true, user true":         {"true", map[string]string{user: "true"}, "", project},
		"project without it, user true":   {"", map[string]string{user: "true"}, "", project},
		"project without it, user false":  {"", map[string]string{user: "false"}, "", off},
		"project true, user false":        {"true", map[string]string{user: "false"}, "", project},
		"local true":                      {"false", map[string]string{local: "true"}, "", localOn},
		"local true, another variable":    {"false", map[string]string{local: "true"}, "yes", localOn},
		"local true over a project true":  {"true", map[string]string{local: "true"}, "2", localOn},
		"local without it":                {"false", map[string]string{local: ""}, "", off},
		"variable 0":                      {"false", map[string]string{local: "false"}, "0", variable},
		"variable 0 over every setting":   {"false", map[string]string{local: "false", user: "false"}, "0", variable},
		"variable 1":                      {"true", map[string]string{local: "true", user: "true"}, "1", off},
		"project true, local false":       {"true", map[string]string{local: "false"}, "", off},
		"another variable, project false": {"false", nil, "true", off},
	} {
		out := Decide(withMemory(t, c.project, c.others, c.env), false)
		if got := memoryOf(out); !slices.Equal(got, []setup.Finding{c.want}) {
			t.Errorf("%s: %+v", name, got)
		}
		if c.want.Level == setup.LevelWarn && (out.Failed || Decide(withMemory(t, c.project, c.others, c.env), true).Failed) {
			t.Errorf("%s: a warning of the area memory never fails the doctor", name)
		}
	}
}

// The area memory has no line without settings that register polix, with settings that cannot be read or are not
// JSON, and outside a repository, even when the settings of the user register polix.
func TestDecideAutoMemoryNone(t *testing.T) {
	s := healthy(t).Setup
	files := s.ClaudeSettingsFiles()
	local, user := files[1], files[2]
	none := withMemory(t, "true", nil, "0")
	none.ClaudeHooks[s.HooksFile(setup.Claude)] = setup.HooksFile{AutoMemory: "true"}
	unread := withMemory(t, "true", map[string]string{local: ""}, "")
	unread.ClaudeHooks[local] = setup.HooksFile{ReadErr: errors.New("permission denied")}
	broken := withMemory(t, "true", nil, "")
	broken.ClaudeHooks[user] = setup.HooksFile{Err: errors.New("not JSON")}
	outside := withMemory(t, "true", nil, "")
	outside.ClaudeHooks = map[string]setup.HooksFile{user: hooksOf(exampleSettings(t))}
	outside.Setup.Root, outside.RepoErr = "", errors.New("not a git repository")
	for name, i := range map[string]setup.Inspection{"no settings register polix": none,
		"local settings that cannot be read": unread, "user settings that are not JSON": broken, "outside a repository": outside} {
		if got := memoryOf(Decide(i, false)); got != nil {
			t.Errorf("%s: %+v", name, got)
		}
	}
}
