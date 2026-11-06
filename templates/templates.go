// Package templates embeds the templates of the texts polix injects into the agents, so they are written as files
// and not as Go strings. They are text/template, rendered by the domain that owns their data.
package templates

import _ "embed"

// SessionStart is the context injected when a session starts: the guidance on commits and edits and the layout of
// the folders of the AI, each part behind its switch of guidance in polix.yaml. It is rendered by
// workflow.Workflow.SessionContext, whose data the template documents.
//
//go:embed session_start.md.tmpl
var SessionStart string
