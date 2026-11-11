# LoadWorkflow

Loads polix.yaml, so every use case that reads the workflow shares one reading of its format and one result per file
and version of polix, and decides on its own what an absent file, a workflow with problems or a file that
cannot be used means to it.

## Trigger

Not a command: `cmd/polix` (`main`) calls `Command` once, which returns the loader, a `workflow.Load`, and passes it
to the `Command` of every use case that reads polix.yaml: `AnswerHook`, `AnswerGithook`, `CheckRule`, `CheckSetup`,
`DiagnoseSetup`, `GraphWorkflow`, `MeasureDiff` and `UpdateVersion`. Each puts it in a field of its adapter, `Workflow`, which calls
it from the port method that loads the workflow (`LoadWorkflow`, or `WorkflowRejected` in `UpdateVersion`) with the
file of the workflow, the polix.yaml at the root of the repository it opened (`filepath.Join(root, workflow.File)`)
or, for `CheckSetup`, the file its argument names, and the version of polix it runs as:

```
workflow(ctx, path, running) workflow.Loaded
```

## Request

`Request`, filled by the loader from its arguments:

- `Path`, the file of the workflow, polix.yaml of a repository or another one, with the separators of the system.
- `Running`, the version of the running polix, which decides whether polix.yaml requires a newer one.

## Logic

`Decide(wf workflow.Workflow, guardrails workflow.Guardrails, exists bool, err error) Output` validates what was read
with the domain:

1. Without the file (and without `err`), the default workflow: version 1, `max_stop_retries` 3
   (`workflow.DefaultMaxStopRetries`) and the default guidance (commit on, edit and layout off), with the error of
   its `Valid`.
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

- `ReadWorkflow`: reads the workflow file at the path through `internal/platform/file` and decodes it for the version of
  polix running; whether the file exists. A missing file has no workflow and no error; a file that cannot be read is
  the error. The decoding, in this order:
  1. `requires`, read before the rest: a release of polix older than it is a `workflow.TooOldError`, which an older
     polix reports instead of the options it does not know; a development build is never too old.
  2. Broken YAML is the error, with the file (`polix.yaml: ...`).
  3. An unknown top level key (not one of `workflow.TopKeys`) is a `workflow.UnknownKeyError`, with the new spelling of
     a key of the old vocabulary.
  4. The decoding types of `out_workflow.go` (`workflowYAML` and the type of each section, entry and verb), with the
     `UnmarshalYAML` of `out_unmarshal.go`, decode the rest: the short and long forms of each verb (`notify` takes
     its message, or a map with `message` and `always`, into `workflow.Notify`), the keys each
     map uses, the line of each entry, of each event of `on` and of `githooks` and of each runner, the runners of
     `rules.run` and the weights of `risk.weights` in file order. The section `githooks`, the gates git runs on every
     commit, is decoded with the decoding types of the entries of `on` into `Workflow.Githooks`, by event, with the
     line of each event in `Workflow.GithookLines`; its entries are never expanded nor mixed with those of `on`, and
     `Workflow.Valid` checks them with the vocabulary of githooks (step 7 only adds one entry of its own to `on`). A value of the wrong kind is an error with its line, and the errors of
     yaml name the types of the domain (`workflow.Workflow`, `workflow.Entry`...), never those of the decoding.
     `max_stop_retries` left out is `workflow.DefaultMaxStopRetries`; commit left out of guidance is on, edit and
     layout left out are off.
     `requirementsYAML` of `out_requirements.go` reads the key `requirements` into `Workflow.Requirements`, the
     programs the repository requires on the PATH, in file order: only a list of names, else an error at the line of
     the node with the form it takes (`line 3: requirements takes a list of program names, as [go, golangci-lint]`);
     a null item is an empty name, which `Workflow.Valid` reports with the names that have spaces around them, are
     paths or are repeated, and left out or empty there is no requirement.
  5. `guardrailsYAML` of `out_guardrails.go` reads the section guardrails into a `workflow.Guardrails`: each
     shortcut in its one form, else an error with the form it takes; each item with its line and the problems of
     its values (a key unknown or written twice, a text that is not one, no pattern, no run, a `wait_agents` that is
     not true or false). An item of `run_on_stop` waits for the subagents still running
     (`workflow.GuardrailCheck.Waits`) unless its `wait_agents` is `false`; one of `format_on_edit` never does, and
     `wait_agents` is an unknown key there.
  6. The workflow takes the protections of the section (`Guardrails.Protected`, `Guardrails.Forbidden`) and the
     entries its other shortcuts expand into, before those written in `on` (`Guardrails.Expand`): an item of
     `run_on_stop` that waits runs only under `agents_are_running: false`, and, when one waits, a `notify` with
     `always` goes last on `stop`, under `agents_are_running: true` and a `changed` with the globs of every item
     that waits (none when one of them has none), with the text `run_on_stop waits for the subagents still running:
     its checks run on the next stop after they end.`; the error is the one of its `Valid`.
  7. When an event of `githooks` has entries, the workflow takes one more entry first in `start`, before those of
     the shortcuts and those written in `on` (`Workflow.ExpandGithooks`): a `run` of `polix install --githooks` with
     `always`, so every session start puts back the hooks of git, with the message `The hooks of git of githooks are
     not installed, so the gates of githooks do not apply to the commits:`, its `From` `githooks` and the line of
     the first event of `githooks` with entries. Without entries in `githooks` the workflow stays as it is; every
     reader of the workflow (`polix hook`, `polix graph`, `polix config check`) sees the entry, while
     `polix config fmt`, which formats the file as written, never writes it.

## Errors

None: the loader never fails. What stops the reading and the problems of the workflow are `Err` of the result, which
each use case reports as its own:

- `CheckSetup` and `DiagnoseSetup` take the workflow, whether it exists and `Err`, and report every problem.
- `AnswerHook` takes no workflow without the file, and else the workflow and `Err`, which its logic answers as a
  workflow it cannot use.
- `AnswerGithook` takes no workflow without the file, and else the workflow and `Err`, with which its logic aborts
  every commit.
- `GraphWorkflow` and `CheckRule` take the workflow of a file with problems as it is, and stop at any other error.
- `MeasureDiff` takes the workflow and `Err`.
- `UpdateVersion` takes `Err` only, to know whether this binary rejects the workflow.

## Flow

1. `Command` builds the loader around `Adapter` as the `Port`, once per process.
2. The first call for a file and a version turns them into a `Request` and runs the shell: it reads polix.yaml
   through the port, calls `Decide` once and builds the response.
3. The loader keeps the result for that file and version for the life of the process, and returns it to every
   later call without reading again: a use case that wrote polix.yaml and read it again in the same process would
   see the old one, which no use case does.
