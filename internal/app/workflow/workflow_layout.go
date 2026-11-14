package workflow

import (
	"path"
	"regexp"
	"strings"
)

// folderPath is the path of the folder of the AI as polix.yaml writes it: the top-level key path.
type folderPath string

// The default folder of the AI, relative to the root, and the folder of the rules below it.
const (
	DefaultPolixPath = "ai"
	DefaultRulesPath = DefaultPolixPath + "/rules"
)

// layoutDrive matches a path that starts with a drive of Windows (C: or C:\).
var layoutDrive = regexp.MustCompile(`^[A-Za-z]:`)

// PolixPath is the folder of the AI, relative to the root, with slashes and no trailing slash: path of polix.yaml
// cleaned, or DefaultPolixPath. It holds the folders rules, plans, reports and tasks.
func (w Workflow) PolixPath() string { return folderPath(w.Path).clean(DefaultPolixPath) }

// RulesPath is the folder of the rules, as PolixPath: rules below PolixPath, DefaultRulesPath without path.
func (w Workflow) RulesPath() string { return w.PolixPath() + "/rules" }

// ValidPath tells why path cannot be the folder of the AI (absolute, the root itself, outside the root or a glob),
// in the words of Workflow.Problems; the rest of the workflow is left to Valid.
func (w Workflow) ValidPath() error {
	v := &validator{}
	v.polixPath(w.Path)
	if len(v.problems) > 0 {
		return InvalidError(v.problems)
	}
	return nil
}

// clean is the path cleaned, with slashes and without ./ nor a trailing slash; def when it is empty.
func (f folderPath) clean(def string) string {
	p := strings.TrimSpace(string(f))
	if p == "" {
		return def
	}
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, `\`, "/")), "./")
}

// problem tells why the path cannot be the folder of the AI: absolute, the root itself, outside the root or with
// wildcards; empty when it can or when it is empty (the default).
func (f folderPath) problem() string {
	p := strings.TrimSpace(string(f))
	if p == "" {
		return ""
	}
	clean := f.clean("")
	switch {
	case path.IsAbs(clean) || layoutDrive.MatchString(p):
		return "must be relative to the root, not absolute"
	case clean == ".":
		return "cannot be the root itself"
	case clean == ".." || strings.HasPrefix(clean, "../"):
		return "must be below the root"
	case strings.ContainsAny(clean, "*?[{"):
		return "must be a folder, not a glob"
	}
	return ""
}

// polixPath reports the problem of path, the one text of it.
func (v *validator) polixPath(p string) {
	if problem := folderPath(p).problem(); problem != "" {
		v.add(0, "path %q %s", p, problem)
	}
}
