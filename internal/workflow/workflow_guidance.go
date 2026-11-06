package workflow

import (
	"strings"
	"text/template"

	"github.com/mlophez/polix-ai/templates"
)

// Guidance is the key `guidance` of polix.yaml: the built-in instructions polix injects into the agent when a
// session starts, each one turned on or off. They ask the agent to work in the forms the heuristics of polix
// recognise and tell it where the folders of the AI are; they block nothing.
type Guidance struct {
	Commit bool     // run each commit as a git commit of its own, so pre_commit recognises it
	Edit   bool     // change files with the edit tool of the agent, so pre_edit and post_edit see them
	Layout bool     // the folders of the rules, the plans and the reports, as polix.yaml sets them
	Keys   []string // the keys the file used, so Valid rejects unknown ones
}

// sessionContextData is the data of the template templates.SessionStart: the switches of the guidance and the
// folders of the layout, relative to the root, without a trailing slash. Its fields are exported for text/template.
type sessionContextData struct {
	Commit, Edit, Layout  bool
	Rules, Plans, Reports string
}

// GuidanceKeys are the keys of guidance, in the order their texts are injected.
var GuidanceKeys = []string{"commit", "edit", "layout"}

// Valid tells why the guidance is not valid: an unknown key.
func (g Guidance) Valid() error {
	v := &validator{}
	v.guidance(g)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// SessionContext renders templates.SessionStart with the guidance and the layout of the workflow: the context
// injected when a session starts. It is empty when every switch is off. The template is embedded in the binary
// and its tests render it, so it never fails here; if it did, the context would be empty.
func (w Workflow) SessionContext() string {
	tmpl, err := template.New("session_start").Option("missingkey=error").Parse(templates.SessionStart)
	if err != nil {
		return ""
	}
	var b strings.Builder
	data := sessionContextData{
		Commit: w.Guidance.Commit, Edit: w.Guidance.Edit, Layout: w.Guidance.Layout,
		Rules: w.RulesPath(), Plans: w.PlansPath(), Reports: w.ReportsPath(),
	}
	if err := tmpl.Execute(&b, data); err != nil {
		return ""
	}
	return strings.TrimSpace(b.String())
}

func (v *validator) guidance(g Guidance) {
	v.unknownKeys(0, "guidance", g.Keys, GuidanceKeys)
}
