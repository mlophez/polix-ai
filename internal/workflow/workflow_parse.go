package workflow

import (
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mlophez/polix-ai/pkg/semver"
)

// TooOld is the error of a workflow that requires a newer polix than the one running.
type TooOld struct {
	Requires string // the version polix.yaml requires
	Running  string // the version of the running polix
}

// workflowYAML is the shape of polix.yaml.
type workflowYAML struct {
	Version       int                    `yaml:"version"`
	Requires      string                 `yaml:"requires"`
	Base          stringsYAML            `yaml:"base"`
	MaxStopBlocks int                    `yaml:"max_stop_blocks"`
	Protected     []string               `yaml:"protected"`
	Vars          map[string]varYAML     `yaml:"vars"`
	On            map[string][]entryYAML `yaml:"on"`
	Rules         rulesYAML              `yaml:"rules"`
	Plans         folderYAML             `yaml:"plans"`
	Reports       folderYAML             `yaml:"reports"`
	Guidance      guidanceYAML           `yaml:"guidance"`
}

// varYAML is a variable of vars.
type varYAML struct {
	Branch string `yaml:"branch"`
	keys   []string
}

// entryYAML is an entry of an event.
type entryYAML struct {
	If      map[string]any `yaml:"if"`
	Unless  map[string]any `yaml:"unless"`
	Mode    string         `yaml:"mode"`
	Why     string         `yaml:"why"`
	Deny    *string        `yaml:"deny"`
	Allow   *bool          `yaml:"allow"`
	Require *string        `yaml:"require"`
	Run     *runYAML       `yaml:"run"`
	Context *contextYAML   `yaml:"context"`
	Record  *string        `yaml:"record"`
	line    int
	keys    []string
}

// runYAML is the action run.
type runYAML struct {
	Command argsYAML `yaml:"command"`
	Rules   string   `yaml:"rules"`
	Record  string   `yaml:"record"`
	keys    []string
}

// contextYAML is the action context.
type contextYAML struct {
	File  string `yaml:"file"`
	Text  string `yaml:"text"`
	Rules string `yaml:"rules"`
	keys  []string
}

// rulesYAML is the key rules.
type rulesYAML struct {
	Path string      `yaml:"path"`
	Run  runnersYAML `yaml:"run"`
	keys []string
}

// folderYAML is the key plans or reports.
type folderYAML struct {
	Path string `yaml:"path"`
	keys []string
}

// guidanceYAML is the key guidance: a switch left out is on.
type guidanceYAML struct {
	Commit *bool `yaml:"commit"`
	Edit   *bool `yaml:"edit"`
	Layout *bool `yaml:"layout"`
	keys   []string
}

// runnerYAML is one runner of rules.run: a glob and its command.
type runnerYAML struct {
	glob string
	argv []string
	line int
}

// stringsYAML is one string or a list of strings.
type stringsYAML []string

// argsYAML is a command as a list of arguments; a string is split on spaces.
type argsYAML []string

// runnersYAML is the map of rules.run, in file order.
type runnersYAML []runnerYAML

// decodeNames translates the names of the decoding types in the errors of yaml into those of the domain, as
// the messages always named them.
var decodeNames = []string{
	"workflow.workflowYAML", "workflow.Workflow",
	"workflow.entryYAML", "workflow.Entry",
	"workflow.varYAML", "workflow.Var",
	"workflow.runYAML", "workflow.Run",
	"workflow.contextYAML", "workflow.Context",
	"workflow.rulesYAML", "workflow.Rules",
	"workflow.folderYAML", "workflow.Folder",
	"workflow.guidanceYAML", "workflow.Guidance",
	"workflow.stringsYAML", "[]string",
	"workflow.argsYAML", "[]string",
}

// topKeys are the keys of polix.yaml.
var topKeys = []string{"version", "requires", "base", "max_stop_blocks", "protected", "vars", "on", "rules", "plans",
	"reports", "guidance"}

// Valid tells why the error is not valid: it names both versions.
func (e TooOld) Valid() error {
	if e.Requires == "" || e.Running == "" {
		return errors.New("too old needs the version required and the running one")
	}
	return nil
}

func (e TooOld) Error() string {
	return fmt.Sprintf("%s requires polix %s or newer and this is %s: run polix update", File, e.Requires, e.Running)
}

// Parse decodes polix.yaml for the polix of version running. No data (nil) is a repository without polix.yaml:
// the default workflow. It returns a TooOld when the workflow requires a newer polix than running, read before
// the rest, which an older polix may not know (a development build is never too old); an error for broken YAML
// or an unknown top level key; and else the workflow with the Invalid error of Valid when it has problems, so
// the caller can still use it.
func Parse(data []byte, running string) (Workflow, error) {
	if data == nil {
		guidance, _ := guidanceYAML{}.guidance()
		w := Workflow{Version: 1, MaxStopBlocks: DefaultMaxStopBlocks, Guidance: guidance}
		return w, w.Valid()
	}
	if req := requires(data); semver.IsRelease(running) && semver.Valid(req) && semver.Compare(running, req) < 0 {
		old := TooOld{Requires: strings.TrimPrefix(req, "v"), Running: running}
		if err := old.Valid(); err != nil {
			return Workflow{}, err
		}
		return Workflow{}, old
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return Workflow{}, fmt.Errorf("%s: %w", File, err)
	}
	if len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode {
		top := root.Content[0]
		for i := 0; i < len(top.Content); i += 2 {
			if k := top.Content[i].Value; !contains(topKeys, k) {
				return Workflow{}, fmt.Errorf("%s:%d: unknown key %q", File, top.Content[i].Line, k)
			}
		}
	}
	var y workflowYAML
	if err := root.Decode(&y); err != nil {
		return Workflow{}, fmt.Errorf("%s: %s", File, strings.NewReplacer(decodeNames...).Replace(err.Error()))
	}
	if y.MaxStopBlocks == 0 {
		y.MaxStopBlocks = DefaultMaxStopBlocks
	}
	return y.workflow()
}

// workflow builds the domain value of the file. The problems of each value are reported by Workflow.Valid,
// which walks them all together with those that depend on the event and the vars, so the errors of the parts
// are not returned on their own.
func (y workflowYAML) workflow() (Workflow, error) {
	var vars map[string]Var
	if y.Vars != nil {
		vars = make(map[string]Var, len(y.Vars))
		for name, v := range y.Vars {
			vars[name], _ = v.variable()
		}
	}
	var on map[string][]Entry
	if y.On != nil {
		on = make(map[string][]Entry, len(y.On))
		for ev, list := range y.On {
			entries := make([]Entry, 0, len(list))
			for _, e := range list {
				entry, _ := e.entry()
				entries = append(entries, entry)
			}
			on[ev] = entries
		}
	}
	rules, _ := y.Rules.rules()
	plans, _ := y.Plans.folder()
	reports, _ := y.Reports.folder()
	guidance, _ := y.Guidance.guidance()
	w := Workflow{
		Version:       y.Version,
		Requires:      y.Requires,
		Base:          []string(y.Base),
		MaxStopBlocks: y.MaxStopBlocks,
		Protected:     y.Protected,
		Vars:          vars,
		On:            on,
		Rules:         rules,
		Plans:         plans,
		Reports:       reports,
		Guidance:      guidance,
	}
	return w, w.Valid()
}

func (y varYAML) variable() (Var, error) {
	v := Var{Branch: y.Branch, Keys: y.keys}
	return v, v.Valid()
}

func (y entryYAML) entry() (Entry, error) {
	var run *Run
	if y.Run != nil {
		r, _ := y.Run.run()
		run = &r
	}
	var context *Context
	if y.Context != nil {
		c, _ := y.Context.context()
		context = &c
	}
	e := Entry{
		If: Cond(y.If), Unless: Cond(y.Unless), Mode: y.Mode, Why: y.Why, Deny: y.Deny, Allow: y.Allow,
		Require: y.Require, Run: run, Context: context, Record: y.Record, Line: y.line, Keys: y.keys,
	}
	return e, e.Valid()
}

func (y runYAML) run() (Run, error) {
	r := Run{Command: []string(y.Command), Rules: y.Rules, Record: y.Record, Keys: y.keys}
	return r, r.Valid()
}

func (y contextYAML) context() (Context, error) {
	c := Context{File: y.File, Text: y.Text, Rules: y.Rules, Keys: y.keys}
	return c, c.Valid()
}

func (y rulesYAML) rules() (Rules, error) {
	runners := make([]Runner, 0, len(y.Run))
	for _, ry := range y.Run {
		r, _ := ry.runner()
		runners = append(runners, r)
	}
	r := Rules{Path: y.Path, Run: runners, Keys: y.keys}
	return r, r.Valid()
}

func (y folderYAML) folder() (Folder, error) {
	f := Folder{Path: y.Path, Keys: y.keys}
	return f, f.Valid()
}

// guidance builds the guidance of the file, with every switch left out turned on.
func (y guidanceYAML) guidance() (Guidance, error) {
	g := Guidance{Commit: y.Commit == nil || *y.Commit, Edit: y.Edit == nil || *y.Edit,
		Layout: y.Layout == nil || *y.Layout, Keys: y.keys}
	return g, g.Valid()
}

func (y runnerYAML) runner() (Runner, error) {
	r := Runner{Glob: y.glob, Argv: y.argv, Line: y.line}
	return r, r.Valid()
}

// UnmarshalYAML accepts a string or a list of strings.
func (ns *stringsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*ns = stringsYAML{n.Value}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*ns = list
	return nil
}

// UnmarshalYAML accepts a string, split on spaces, or a list of strings.
func (a *argsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*a = strings.Fields(n.Value)
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*a = list
	return nil
}

// UnmarshalYAML keeps the line of the entry and the keys it uses, so Valid can reject unknown ones.
func (y *entryYAML) UnmarshalYAML(n *yaml.Node) error {
	type plain entryYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = entryYAML(p)
	y.line = n.Line
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of plans or reports, so Valid can reject unknown ones.
func (y *folderYAML) UnmarshalYAML(n *yaml.Node) error {
	type plain folderYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = folderYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of the variable, so Valid can reject unknown ones.
func (y *varYAML) UnmarshalYAML(n *yaml.Node) error {
	type plain varYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = varYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of the run, so Valid can reject unknown ones.
func (y *runYAML) UnmarshalYAML(n *yaml.Node) error {
	type plain runYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = runYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of the context, so Valid can reject unknown ones.
func (y *contextYAML) UnmarshalYAML(n *yaml.Node) error {
	type plain contextYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = contextYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of rules, so Valid can reject unknown ones.
func (y *rulesYAML) UnmarshalYAML(n *yaml.Node) error {
	type plain rulesYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = rulesYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML keeps the keys of guidance, so Valid can reject unknown ones.
func (y *guidanceYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: guidance must map commit and edit to true or false", n.Line)
	}
	type plain guidanceYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = guidanceYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML reads the map of rules.run in file order: a glob to a command, a string split on spaces or a
// list of strings.
func (rs *runnersYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: rules.run must map a glob to a command", n.Line)
	}
	var out runnersYAML
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		if key.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a glob of rules.run must be a string", key.Line)
		}
		if first, ok := seen[key.Value]; ok {
			return fmt.Errorf("line %d: mapping key %q already defined at line %d", key.Line, key.Value, first)
		}
		seen[key.Value] = key.Line
		var argv argsYAML
		if err := n.Content[i+1].Decode(&argv); err != nil {
			return err
		}
		out = append(out, runnerYAML{glob: n.Content[i].Value, argv: argv, line: n.Content[i].Line})
	}
	*rs = out
	return nil
}

// mapKeys returns the keys of a mapping node, in file order.
func mapKeys(n *yaml.Node) []string {
	var keys []string
	if n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			keys = append(keys, n.Content[i].Value)
		}
	}
	return keys
}

// requires reads the top level `requires` of a workflow without decoding the rest; "" when it has none or the
// YAML is broken (Parse reports it then).
func requires(data []byte) string {
	var root yaml.Node
	if yaml.Unmarshal(data, &root) != nil || len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return ""
	}
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value == "requires" && top.Content[i+1].Kind == yaml.ScalarNode {
			return strings.TrimSpace(top.Content[i+1].Value)
		}
	}
	return ""
}
