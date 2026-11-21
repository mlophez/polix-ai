package workflow

import (
	"strings"
	"text/template"

	"github.com/mlophez/polix-ai/templates"
)

// sessionContextData is the data of the template templates.SessionStart: the root of the working tree, with forward
// slashes, and the folder of the AI, relative to the root, without a trailing slash. Its fields are exported for
// text/template.
type sessionContextData struct {
	Root, Path string
}

// SessionContext renders templates.SessionStart with the root of the working tree and the folder of the AI of the
// workflow (PolixPath): the context polix injects on every start of a session, which asks the agent to commit in
// the forms pre_commit recognises and tells it where the folders of the AI are; it blocks nothing. The root is shown
// with forward slashes on every system. The template is embedded in the binary and its tests render it, so it never
// fails here; if it did, the context would be empty.
func (w Workflow) SessionContext(root string) string {
	tmpl, err := template.New("session_start").Option("missingkey=error").Parse(templates.SessionStart)
	if err != nil {
		return ""
	}
	var b strings.Builder
	data := sessionContextData{Root: strings.ReplaceAll(root, `\`, "/"), Path: w.PolixPath()}
	if err := tmpl.Execute(&b, data); err != nil {
		return ""
	}
	return strings.TrimSpace(b.String())
}
