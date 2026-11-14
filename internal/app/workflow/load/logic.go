package load

import "github.com/mlophez/polix-ai/internal/app/workflow"

// Output is the decision of LoadWorkflow: what polix.yaml loads.
type Output struct {
	Loaded workflow.Loaded
}

// Decide validates what was read from polix.yaml with the domain. Without the file, it is the default workflow;
// with an error that stops the reading, no workflow and that error. Else it is the workflow read, with its problems
// (err, the InvalidError of Workflow.Valid) or, when the values of the shortcuts of guardrails have problems, those
// problems together with the rest in the order of the file (Guardrails.Problems), as one Invalid.
func Decide(wf workflow.Workflow, guardrails workflow.Guardrails, exists bool, err error) Output {
	if !exists && err == nil {
		wf, err = absent()
	}
	if p := guardrails.Problems(wf); len(p) > 0 {
		err = workflow.InvalidError(p)
	}
	loaded := workflow.Loaded{Workflow: wf, Exists: exists, Err: err}
	if verr := loaded.Valid(); verr != nil {
		return Output{Loaded: loaded.Fail(verr)}
	}
	return Output{Loaded: loaded}
}

// absent is the workflow of a repository without polix.yaml: version 1, the default loop guard and the default
// folder of the AI, with the error of its Valid.
func absent() (workflow.Workflow, error) {
	w := workflow.Workflow{Version: 1, MaxStopRetries: workflow.DefaultMaxStopRetries}
	return w, w.Valid()
}
