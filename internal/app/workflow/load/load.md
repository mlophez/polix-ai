# LoadWorkflow

Loads polix.yaml, so every use case that reads the workflow shares one reading of its format and one result per
repository and version of polix, and decides on its own what an absent file, a workflow with problems or a file that
cannot be used means to it.

## Trigger

Not a command: `cmd/polix` (`main`) calls `Command` once, which returns the loader, a `workflow.Load`, and passes it
to the `Command` of every use case that reads polix.yaml: `AnswerHook`, `CheckRule`, `CheckSetup`, `DiagnoseSetup`,
`GraphWorkflow`, `MeasureDiff` and `UpdateVersion`. Each puts it in a field of its adapter, `Workflow`, which calls
it from the port method that loads the workflow (`LoadWorkflow`, or `WorkflowRejected` in `UpdateVersion`) with the
root of the repository it opened and the version of polix it runs as:

```
workflow(ctx, root, running) workflow.Loaded
```

## Request

`Request`, filled by the loader from its arguments:

- `Root`, the root of the repository, with the separators of the system.
- `Running`, the version of the running polix, which decides whether polix.yaml requires a newer one.

## Logic

`Decide(wf workflow.Workflow, guardrails workflow.Guardrails, exists bool, err error) Output` validates what was read
with the domain:

1. Without the file (and without `err`), the default workflow: version 1, `max_stop_retries` 3
   (`workflow.DefaultMaxStopRetries`) and every switch of guidance on, with the error of its `Valid`.
2. When the values of the shortcuts of guardrails have problems (`Guardrails.Problems`), those problems and the
   rest of the problems of the workflow, in the order of the file, are the error, as one `workflow.InvalidError`;
   otherwise `err` stays: nil, the `workflow.InvalidError` of `Workflow.Valid`, or an error that stops the reading.
3. The result is a `workflow.Loaded`, followed by `Valid`; one that is not valid fails with its error
   (`Loaded.Fail`).

## Response

`Response`: `Loaded`, the `workflow.Loaded` the loader returns: `Workflow`, `Exists` (whether polix.yaml exists) and
`Err`.

## Port

`Port`, implemented by `Adapter` in `out.go` and its siblings:

- `ReadWorkflow`: reads polix.yaml of the root through `internal/platform/file` and decodes it for the version of
  polix running; whether the file exists. A missing file has no workflow and no error; a file that cannot be read is
  the error. The decoding, in this order:
  1. `requires`, read before the rest: a release of polix older than it is a `workflow.TooOldError`, which an older
     polix reports instead of the options it does not know; a development build is never too old.
  2. Broken YAML is the error, with the file (`polix.yaml: ...`).
  3. An unknown top level key (not one of `workflow.TopKeys`) is a `workflow.UnknownKeyError`, with the new spelling of
     a key of the old vocabulary.
  4. The decoding types of `out_workflow.go` (`workflowYAML` and the type of each section, entry and verb), with the
     `UnmarshalYAML` of `out_unmarshal.go`, decode the rest: the short and long forms of each verb, the keys each
     map uses, the line of each entry, of each event of `on` and of each runner, the runners of `rules.run` and the
     weights of `risk.weights` in file order. A value of the wrong kind is an error with its line, and the errors of
     yaml name the types of the domain (`workflow.Workflow`, `workflow.Entry`...), never those of the decoding.
     `max_stop_retries` left out is `workflow.DefaultMaxStopRetries`; a switch of guidance left out is on.
     `requirementsYAML` of `out_requirements.go` reads the key `requirements` into `Workflow.Requirements`, the
     programs the repository requires on the PATH, in file order: only a list of names, else an error at the line of
     the node with the form it takes (`line 3: requirements takes a list of program names, as [go, golangci-lint]`);
     a null item is an empty name, which `Workflow.Valid` reports with the names that have spaces around them, are
     paths or are repeated, and left out or empty there is no requirement.
  5. `guardrailsYAML` of `out_guardrails.go` reads the section guardrails into a `workflow.Guardrails`: each
     shortcut in its one form, else an error with the form it takes; each item with its line and the problems of
     its values (a key unknown or written twice, a text that is not one, no pattern, no run).
  6. The workflow takes the protections of the section (`Guardrails.Protected`, `Guardrails.Forbidden`) and the
     entries its other shortcuts expand into, before those written in `on` (`Guardrails.Expand`); the error is the
     one of its `Valid`.

## Errors

None: the loader never fails. What stops the reading and the problems of the workflow are `Err` of the result, which
each use case reports as its own:

- `CheckSetup` and `DiagnoseSetup` take the workflow, whether it exists and `Err`, and report every problem.
- `AnswerHook` takes no workflow without the file, and else the workflow and `Err`, which its logic answers as a
  workflow it cannot use.
- `GraphWorkflow` and `CheckRule` take the workflow of a file with problems as it is, and stop at any other error.
- `MeasureDiff` takes the workflow and `Err`.
- `UpdateVersion` takes `Err` only, to know whether this binary rejects the workflow.

## Flow

1. `Command` builds the loader around `Adapter` as the `Port`, once per process.
2. The first call for a root and a version turns them into a `Request` and runs the shell: it reads polix.yaml
   through the port, calls `Decide` once and builds the response.
3. The loader keeps the result for that root and version for the life of the process, and returns it to every
   later call without reading again: a use case that wrote polix.yaml and read it again in the same process would
   see the old one, which no use case does.
