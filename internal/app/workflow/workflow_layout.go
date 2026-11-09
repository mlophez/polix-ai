package workflow

import (
	"path"
	"regexp"
	"strings"
)

// Folder is a key of polix.yaml that only places a folder of the AI, `plans` or `reports`: its path relative to
// the root, empty for the default.
type Folder struct {
	Path string
	Keys []string // the keys the file used, so Valid rejects unknown ones
}

// The default folders of the AI, relative to the root.
const (
	DefaultRulesPath   = "ai/rules"
	DefaultPlansPath   = "ai/plans"
	DefaultReportsPath = "ai/reports"
)

// FolderKeys are the keys of plans and reports.
var FolderKeys = []string{"path"}

// layoutDrive matches a path that starts with a drive of Windows (C: or C:\).
var layoutDrive = regexp.MustCompile(`^[A-Za-z]:`)

// Valid tells why the folder is not valid: an unknown key or a path that is not a folder below the root.
func (f Folder) Valid() error {
	v := &validator{}
	v.folder("folder", f)
	if len(v.problems) > 0 {
		return Invalid(v.problems)
	}
	return nil
}

// RulesPath is the folder of the rules, relative to the root, with slashes and no trailing slash: rules.path or
// DefaultRulesPath.
func (w Workflow) RulesPath() string { return layoutPath(w.Rules.Path, DefaultRulesPath) }

// PlansPath is the folder of the plans, as RulesPath: plans.path or DefaultPlansPath.
func (w Workflow) PlansPath() string { return layoutPath(w.Plans.Path, DefaultPlansPath) }

// ReportsPath is the folder of the reports, as RulesPath: reports.path or DefaultReportsPath.
func (w Workflow) ReportsPath() string { return layoutPath(w.Reports.Path, DefaultReportsPath) }

// layoutPath is p cleaned, with slashes and without ./ nor a trailing slash; def when p is empty.
func layoutPath(p, def string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return def
	}
	return strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, `\`, "/")), "./")
}

// layoutProblem tells why p cannot be a folder of the AI: absolute, the root itself, outside the root or with
// wildcards; empty when it can or when p is empty (the default).
func layoutProblem(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	clean := layoutPath(p, "")
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

// folder reports the problems of the folder of the key what.
func (v *validator) folder(what string, f Folder) {
	v.unknownKeys(0, what, f.Keys, FolderKeys)
	if p := layoutProblem(f.Path); p != "" {
		v.add(0, "%s.path %q %s", what, f.Path, p)
	}
}
