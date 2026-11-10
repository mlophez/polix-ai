package load

import (
	"strings"

	"github.com/mlophez/polix-ai/internal/app/workflow"
)

// workflowYAML is the shape of polix.yaml.
type workflowYAML struct {
	Version        int                    `yaml:"version"`
	Requires       string                 `yaml:"requires"`
	BaseBranch     stringsYAML            `yaml:"base_branch"`
	MaxStopRetries int                    `yaml:"max_stop_retries"`
	Requirements   requirementsYAML       `yaml:"requirements"`
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

// names translates the names of the decoding types in the errors of yaml into those of the domain, as the messages
// always named them.
func (workflowYAML) names() *strings.Replacer {
	return strings.NewReplacer(
		"load.workflowYAML", "workflow.Workflow",
		"load.entryYAML", "workflow.Entry",
		"load.varYAML", "workflow.Var",
		"load.actionYAML", "workflow.Action",
		"load.runYAML", "workflow.Run",
		"load.injectYAML", "workflow.Inject",
		"load.blockYAML", "workflow.Block",
		"load.rulesYAML", "workflow.Rules",
		"load.folderYAML", "workflow.Folder",
		"load.guidanceYAML", "workflow.Guidance",
		"load.stringsYAML", "[]string",
		"load.argsYAML", "[]string",
		// The local type each UnmarshalYAML decodes a map into; they check the kind of their node first, so this
		// only keeps a name of the decoding out of a message that slips through.
		"load.plain", "a map",
	)
}

// workflow builds the domain value of the file, with the line of each event of `on`, the protected files and the
// forbidden commands of guardrails, and its other shortcuts expanded into entries, and returns it with the section
// guardrails as written. The problems of each value are reported by Workflow.Valid, which walks them all together
// with those that depend on the event and the vars, so the errors of the parts are not returned on their own; the
// problems of the values of the shortcuts, which the workflow does not hold, are left to Guardrails.Problems.
func (y workflowYAML) workflow(eventLines map[string]int) (workflow.Workflow, workflow.Guardrails, error) {
	guardrails, _ := y.Guardrails.guardrails()
	rules, _ := y.Rules.rules()
	plans, _ := y.Plans.folder()
	reports, _ := y.Reports.folder()
	guidance, _ := y.Guidance.guidance()
	w := workflow.Workflow{
		Weights:           y.Risk.Weights.weights(),
		Version:           y.Version,
		Requires:          y.Requires,
		Base:              []string(y.BaseBranch),
		MaxStopRetries:    y.MaxStopRetries,
		Requirements:      []string(y.Requirements),
		ProtectedFiles:    guardrails.Protected(),
		ForbiddenCommands: guardrails.Forbidden(),
		Vars:              y.vars(),
		On:                guardrails.Expand(y.on()),
		Rules:             rules,
		Plans:             plans,
		Reports:           reports,
		Guidance:          guidance,
		EventLines:        eventLines,
	}
	return w, guardrails, w.Valid()
}

// vars builds the variables of vars; nil without the key.
func (y workflowYAML) vars() map[string]workflow.Var {
	if y.Vars == nil {
		return nil
	}
	vars := make(map[string]workflow.Var, len(y.Vars))
	for name, v := range y.Vars {
		vars[name], _ = v.variable()
	}
	return vars
}

// on builds the entries written in on, by event; nil without the key.
func (y workflowYAML) on() map[string][]workflow.Entry {
	if y.On == nil {
		return nil
	}
	on := make(map[string][]workflow.Entry, len(y.On))
	for ev, list := range y.On {
		entries := make([]workflow.Entry, 0, len(list))
		for _, e := range list {
			entry, _ := e.entry()
			entries = append(entries, entry)
		}
		on[ev] = entries
	}
	return on
}

func (y varYAML) variable() (workflow.Var, error) {
	v := workflow.Var{Branch: y.Branch, Keys: y.keys}
	return v, v.Valid()
}

func (y entryYAML) entry() (workflow.Entry, error) {
	var action workflow.Action
	if y.Action != nil {
		action, _ = y.Action.action()
	}
	e := workflow.Entry{When: workflow.Cond(y.When), Unless: workflow.Cond(y.Unless), Action: action, Line: y.line,
		Keys: y.keys, OldIf: workflow.Cond(y.If)}
	return e, e.Valid()
}

func (y actionYAML) action() (workflow.Action, error) {
	var run *workflow.Run
	if y.Run != nil {
		r, _ := y.Run.run()
		run = &r
	}
	var inject *workflow.Inject
	if y.Inject != nil {
		c, _ := y.Inject.inject()
		inject = &c
	}
	var block *workflow.Block
	if y.Block != nil {
		b, _ := y.Block.block()
		block = &b
	}
	a := workflow.Action{Run: run, Inject: inject, Block: block, Keys: y.keys}
	return a, a.Valid()
}

func (y runYAML) run() (workflow.Run, error) {
	r := workflow.Run{Command: []string(y.Command), Rules: y.Rules, Message: y.Message, Always: y.Always, Keys: y.keys}
	return r, r.Valid()
}

func (y injectYAML) inject() (workflow.Inject, error) {
	c := workflow.Inject{File: y.File, Text: y.Text, Rules: y.Rules, Always: y.Always, Keys: y.keys}
	return c, c.Valid()
}

func (y blockYAML) block() (workflow.Block, error) {
	b := workflow.Block{Message: y.Message, Once: y.Once, Keys: y.keys}
	return b, b.Valid()
}

func (y rulesYAML) rules() (workflow.Rules, error) {
	runners := make([]workflow.Runner, 0, len(y.Run))
	for _, ry := range y.Run {
		r, _ := ry.runner()
		runners = append(runners, r)
	}
	r := workflow.Rules{Path: y.Path, Run: runners, Keys: y.keys}
	return r, r.Valid()
}

func (y folderYAML) folder() (workflow.Folder, error) {
	f := workflow.Folder{Path: y.Path, Keys: y.keys}
	return f, f.Valid()
}

// guidance builds the guidance of the file, with every switch left out turned on.
func (y guidanceYAML) guidance() (workflow.Guidance, error) {
	g := workflow.Guidance{Commit: y.Commit == nil || *y.Commit, Edit: y.Edit == nil || *y.Edit,
		Layout: y.Layout == nil || *y.Layout, Keys: y.keys}
	return g, g.Valid()
}

// weights builds the weights of risk.weights in file order. The problems of each weight are reported by
// Workflow.Valid.
func (ws weightsYAML) weights() []workflow.Weight {
	if len(ws) == 0 {
		return nil
	}
	out := make([]workflow.Weight, 0, len(ws))
	for _, y := range ws {
		w, _ := y.weight()
		out = append(out, w)
	}
	return out
}

func (y weightYAML) weight() (workflow.Weight, error) {
	w := workflow.Weight{Glob: y.glob, Factor: y.factor}
	return w, w.Valid()
}

func (y runnerYAML) runner() (workflow.Runner, error) {
	r := workflow.Runner{Glob: y.glob, Argv: y.argv, Line: y.line}
	return r, r.Valid()
}
