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
	Version        int                    `yaml:"version"`
	Requires       string                 `yaml:"requires"`
	BaseBranch     stringsYAML            `yaml:"base_branch"`
	MaxStopRetries int                    `yaml:"max_stop_retries"`
	Guardrails     guardrailsYAML         `yaml:"guardrails"`
	Vars           map[string]varYAML     `yaml:"vars"`
	On             map[string][]entryYAML `yaml:"on"`
	Rules          rulesYAML              `yaml:"rules"`
	Plans          folderYAML             `yaml:"plans"`
	Reports        folderYAML             `yaml:"reports"`
	Guidance       guidanceYAML           `yaml:"guidance"`
	Risk           riskYAML               `yaml:"risk"`
}

// riskYAML is the key risk: the options of the risk score.
type riskYAML struct {
	Weights weightsYAML `yaml:"weights"`
}

// weightYAML is one weight of risk.weights: a glob and its factor.
type weightYAML struct {
	glob   string
	factor float64
}

// weightsYAML is the map of risk.weights, in file order.
type weightsYAML []weightYAML

// varYAML is a variable of vars.
type varYAML struct {
	Branch string `yaml:"branch"`
	keys   []string
}

// entryYAML is an entry of an event. The keys of the old vocabulary reach the validator as unknown keys, which
// reports their new spelling; only the conditions of if are decoded, so their problems come with that hint.
type entryYAML struct {
	When   map[string]any `yaml:"when"`
	Unless map[string]any `yaml:"unless"`
	If     ifYAML         `yaml:"if"`
	Action *actionYAML    `yaml:"action"`
	line   int
	keys   []string
}

// ifYAML is the value of the old key if: its conditions when it is a map, else nothing, since the hint of the key
// already tells how to rewrite it.
type ifYAML map[string]any

// actionYAML is the action of an entry: a map with one verb.
type actionYAML struct {
	Run    *runYAML    `yaml:"run"`
	Inject *injectYAML `yaml:"inject"`
	Block  *blockYAML  `yaml:"block"`
	keys   []string
}

// runYAML is the verb run: a command (a string split on spaces, or a list) or a map.
type runYAML struct {
	Command argsYAML `yaml:"command"`
	Rules   string   `yaml:"rules"`
	Message string   `yaml:"message"`
	Always  bool     `yaml:"always"`
	keys    []string
}

// injectYAML is the verb inject: a text or a map.
type injectYAML struct {
	File   string `yaml:"file"`
	Text   string `yaml:"text"`
	Rules  string `yaml:"rules"`
	Always bool   `yaml:"always"`
	keys   []string
}

// blockYAML is the verb block: a message or a map.
type blockYAML struct {
	Message string `yaml:"message"`
	Once    bool   `yaml:"once"`
	keys    []string
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
	"workflow.actionYAML", "workflow.Action",
	"workflow.runYAML", "workflow.Run",
	"workflow.injectYAML", "workflow.Inject",
	"workflow.blockYAML", "workflow.Block",
	"workflow.rulesYAML", "workflow.Rules",
	"workflow.folderYAML", "workflow.Folder",
	"workflow.guidanceYAML", "workflow.Guidance",
	"workflow.stringsYAML", "[]string",
	"workflow.argsYAML", "[]string",
	// The local type each UnmarshalYAML decodes a map into; they check the kind of their node first, so this only
	// keeps a name of the decoding out of a message that slips through.
	"workflow.plain", "a map",
}

// topKeys are the keys of polix.yaml.
var topKeys = []string{"version", "requires", "base_branch", "max_stop_retries", "guardrails", "vars", "on", "rules",
	"plans", "reports", "guidance", "risk"}

// riskKeys are the keys of the section risk.
var riskKeys = []string{"weights"}

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
// or an unknown top level key, with the new spelling of a renamed one; and else the workflow with the Invalid
// error of Valid when it has problems, so the caller can still use it.
func Parse(data []byte, running string) (Workflow, error) {
	if data == nil {
		guidance, _ := guidanceYAML{}.guidance()
		w := Workflow{Version: 1, MaxStopRetries: DefaultMaxStopRetries, Guidance: guidance}
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
	var lines map[string]int
	if len(root.Content) == 1 && root.Content[0].Kind == yaml.MappingNode {
		top := root.Content[0]
		for i := 0; i < len(top.Content); i += 2 {
			if k := top.Content[i].Value; !contains(topKeys, k) {
				if hint, ok := renamed["top"][k]; ok {
					return Workflow{}, fmt.Errorf("%s:%d: unknown key %q: %s", File, top.Content[i].Line, k, hint)
				}
				return Workflow{}, fmt.Errorf("%s:%d: unknown key %q", File, top.Content[i].Line, k)
			}
			if top.Content[i].Value == "on" && i+1 < len(top.Content) {
				lines = keyLines(top.Content[i+1])
			}
		}
	}
	var y workflowYAML
	if err := root.Decode(&y); err != nil {
		return Workflow{}, fmt.Errorf("%s: %s", File, strings.NewReplacer(decodeNames...).Replace(err.Error()))
	}
	if y.MaxStopRetries == 0 {
		y.MaxStopRetries = DefaultMaxStopRetries
	}
	return y.workflow(lines)
}

// workflow builds the domain value of the file, with the line of each event of `on`, the protected files and the
// forbidden commands of guardrails, and its other shortcuts expanded into entries. The problems of each value are
// reported by Workflow.Valid, which walks them all together with those that depend on the event and the vars, so the
// errors of the parts are not returned on their own; the problems of the values of the shortcuts, which the workflow
// does not hold, come with them in the order of the file.
func (y workflowYAML) workflow(eventLines map[string]int) (Workflow, error) {
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
	weights := y.Risk.Weights.weights()
	w := Workflow{
		Weights:           weights,
		Version:           y.Version,
		Requires:          y.Requires,
		Base:              []string(y.BaseBranch),
		MaxStopRetries:    y.MaxStopRetries,
		ProtectedFiles:    y.Guardrails.protected(),
		ForbiddenCommands: y.Guardrails.forbidden(),
		Vars:              vars,
		On:                y.Guardrails.expand(on),
		Rules:             rules,
		Plans:             plans,
		Reports:           reports,
		Guidance:          guidance,
		EventLines:        eventLines,
	}
	err := w.Valid()
	if p := y.Guardrails.problems(w); len(p) > 0 {
		err = Invalid(p)
	}
	return w, err
}

func (y varYAML) variable() (Var, error) {
	v := Var{Branch: y.Branch, Keys: y.keys}
	return v, v.Valid()
}

func (y entryYAML) entry() (Entry, error) {
	var action Action
	if y.Action != nil {
		action, _ = y.Action.action()
	}
	e := Entry{When: Cond(y.When), Unless: Cond(y.Unless), Action: action, Line: y.line, Keys: y.keys,
		OldIf: Cond(y.If)}
	return e, e.Valid()
}

func (y actionYAML) action() (Action, error) {
	var run *Run
	if y.Run != nil {
		r, _ := y.Run.run()
		run = &r
	}
	var inject *Inject
	if y.Inject != nil {
		c, _ := y.Inject.inject()
		inject = &c
	}
	var block *Block
	if y.Block != nil {
		b, _ := y.Block.block()
		block = &b
	}
	a := Action{Run: run, Inject: inject, Block: block, Keys: y.keys}
	return a, a.Valid()
}

func (y runYAML) run() (Run, error) {
	r := Run{Command: []string(y.Command), Rules: y.Rules, Message: y.Message, Always: y.Always, Keys: y.keys}
	return r, r.Valid()
}

func (y injectYAML) inject() (Inject, error) {
	c := Inject{File: y.File, Text: y.Text, Rules: y.Rules, Always: y.Always, Keys: y.keys}
	return c, c.Valid()
}

func (y blockYAML) block() (Block, error) {
	b := Block{Message: y.Message, Once: y.Once, Keys: y.keys}
	return b, b.Valid()
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

// weights builds the weights of risk.weights in file order. The problems of each weight are reported by
// Workflow.Valid.
func (ws weightsYAML) weights() []Weight {
	if len(ws) == 0 {
		return nil
	}
	out := make([]Weight, 0, len(ws))
	for _, y := range ws {
		w, _ := y.weight()
		out = append(out, w)
	}
	return out
}

func (y weightYAML) weight() (Weight, error) {
	w := Weight{Glob: y.glob, Factor: y.factor}
	return w, w.Valid()
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
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: an entry must be a map with %s", n.Line, strings.Join(entryKeys, ", "))
	}
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

// UnmarshalYAML reads the conditions of the old key if when it is a map, and ignores any other value.
func (y *ifYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	var c map[string]any
	if err := n.Decode(&c); err != nil {
		return err
	}
	*y = c
	return nil
}

// UnmarshalYAML keeps the keys of plans or reports, so Valid can reject unknown ones.
func (y *folderYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: plans and reports must be a map with path", n.Line)
	}
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
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a variable of vars must be a map with branch", n.Line)
	}
	type plain varYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = varYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML reads the action, a map, and keeps its keys, so Valid can reject unknown ones.
func (y *actionYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: action must be a map with one of %s", n.Line, strings.Join(Actions, ", "))
	}
	type plain actionYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = actionYAML(p)
	y.keys = mapKeys(n)
	return nil
}

// UnmarshalYAML reads the run: a string split on spaces or a list of arguments is the command, and a map keeps its
// keys, so Valid can reject unknown ones.
func (y *runYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode, yaml.SequenceNode:
		var args argsYAML
		if err := n.Decode(&args); err != nil {
			return err
		}
		*y = runYAML{Command: args}
		return nil
	case yaml.MappingNode:
		if err := boolKeys(n, VerbRun, "always"); err != nil {
			return err
		}
		type plain runYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = runYAML(p)
		y.keys = mapKeys(n)
		return nil
	}
	return fmt.Errorf("line %d: run takes a command, a list of arguments or a map with command or rules", n.Line)
}

// UnmarshalYAML reads the inject: a string is the text, and a map keeps its keys, so Valid can reject unknown ones.
func (y *injectYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*y = injectYAML{Text: n.Value}
		return nil
	case yaml.MappingNode:
		if err := boolKeys(n, VerbInject, "always"); err != nil {
			return err
		}
		type plain injectYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = injectYAML(p)
		y.keys = mapKeys(n)
		return nil
	}
	return fmt.Errorf("line %d: inject takes a text or a map with one of file, text or rules", n.Line)
}

// UnmarshalYAML reads the block: a string is the message, and a map keeps its keys, so Valid can reject unknown
// ones.
func (y *blockYAML) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		*y = blockYAML{Message: n.Value}
		return nil
	case yaml.MappingNode:
		if err := boolKeys(n, VerbBlock, "once"); err != nil {
			return err
		}
		type plain blockYAML
		var p plain
		if err := n.Decode(&p); err != nil {
			return err
		}
		*y = blockYAML(p)
		y.keys = mapKeys(n)
		return nil
	}
	return fmt.Errorf("line %d: block takes a message or a map with message and once", n.Line)
}

// UnmarshalYAML keeps the keys of rules, so Valid can reject unknown ones.
func (y *rulesYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: rules must be a map with path and run", n.Line)
	}
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

// UnmarshalYAML reads the section risk, a map whose only key is weights.
func (y *riskYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: risk must be a map with weights", n.Line)
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i]; !contains(riskKeys, k.Value) {
			return fmt.Errorf("line %d: unknown key %q in risk (%s)", k.Line, k.Value, strings.Join(riskKeys, ", "))
		}
	}
	type plain riskYAML
	var p plain
	if err := n.Decode(&p); err != nil {
		return err
	}
	*y = riskYAML(p)
	return nil
}

// UnmarshalYAML reads the map of risk.weights in file order: a glob to a number.
func (ws *weightsYAML) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: risk.weights must map a glob to a number", n.Line)
	}
	var out weightsYAML
	seen := map[string]int{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return fmt.Errorf("line %d: a glob of risk.weights must be a string", key.Line)
		}
		if first, ok := seen[key.Value]; ok {
			return fmt.Errorf("line %d: mapping key %q already defined at line %d", key.Line, key.Value, first)
		}
		seen[key.Value] = key.Line
		var f float64
		if val.Kind != yaml.ScalarNode || (val.Tag != "!!int" && val.Tag != "!!float") || val.Decode(&f) != nil {
			return fmt.Errorf("line %d: the weight of %q must be a number", val.Line, key.Value)
		}
		out = append(out, weightYAML{glob: key.Value, factor: f})
	}
	*ws = out
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

// keyLines returns the line of each key of a mapping node.
func keyLines(n *yaml.Node) map[string]int {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	lines := make(map[string]int, len(n.Content)/2)
	for i := 0; i < len(n.Content); i += 2 {
		lines[n.Content[i].Value] = n.Content[i].Line
	}
	return lines
}

// boolKeys returns the error of a key of the map n, of the verb what, among keys whose value is not true or false.
func boolKeys(n *yaml.Node, what string, keys ...string) error {
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, val := n.Content[i], n.Content[i+1]
		if contains(keys, k.Value) && (val.Kind != yaml.ScalarNode || val.ShortTag() != "!!bool") {
			return fmt.Errorf("line %d: %s.%s must be true or false", val.Line, what, k.Value)
		}
	}
	return nil
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
