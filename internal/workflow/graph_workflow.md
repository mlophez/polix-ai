# GraphWorkflow

Prints the workflow of polix.yaml as a Mermaid flowchart, so a person sees at a glance what polix does on each
event.

## Trigger

```
polix graph [--dir <dir>]
```

- `--dir`: any folder of the repository; defaults to the current one.

## Request

`GraphWorkflowRequest`, filled by the command from its flags:

- `Dir`, from `--dir`.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`GraphWorkflow(wf Workflow, found bool) GraphWorkflowOutput` draws the workflow. It takes what the shell read
through the port:

- `wf`: the workflow of polix.yaml, drawn as it is even when `polix check` would report problems.
- `found`: whether polix.yaml exists.

`GraphWorkflowOutput` holds `Mermaid`, the flowchart, and `Missing`, set when there is no polix.yaml. The rules
it follows:

1. Without polix.yaml there is nothing to draw: `Missing`, and no flowchart.
2. The flowchart starts with `flowchart TD` and has one subgraph per event with entries, in the order of
   `Events`, each closed by `end`.
3. Each entry is a node `n<k>`, numbered across the whole flowchart, linked to the previous entry of its event
   with `-->`.
4. The label of a node joins with `<br/>` its `when` and `unless` conditions, as `when key=value` and
   `unless key=value` ordered by key, and the verb of its action: `block: <message>`, followed by `(once)` with
   `once: true`; `run` with its command or `rules <target>`, and `inject` with `file <file>`, `rules <target>` or
   `: <text>`, each followed by `(always)` with `always: true`. Double quotes become single ones.

## Response

`GraphWorkflowResponse`, from the output: `Mermaid`, the flowchart, and `Missing`, when there is no polix.yaml.
The command prints the flowchart to stdout and exits with 0; without polix.yaml it fails with
`polix.yaml not found` and exits with 1.

## Port

`GraphWorkflowPort`, declared in `graph_workflow_in.go` and implemented by `GraphWorkflowAdapter` in
`graph_workflow_out.go`:

- `LoadWorkflow`: reads polix.yaml and tells whether it exists; without it, the default workflow. The problems of
  a workflow that parses (`Invalid`) are not an error: the workflow is drawn as it is.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- polix.yaml cannot be read, is not valid yaml, does not decode into the schema of polix.yaml (a malformed
  `rules.run` among them), has an unknown top level key or requires a newer polix.
- There is no polix.yaml.

## Flow

1. `GraphWorkflowCommand` turns the flags into a `GraphWorkflowRequest` and injects
   `GraphWorkflowAdapter{Repo: repo}` as the `GraphWorkflowPort`.
2. The shell loads the workflow through the port.
3. `GraphWorkflow` draws it: `GraphWorkflowOutput`.
4. The shell builds the response from the output.
5. `GraphWorkflowCommand` fails without polix.yaml, else prints the flowchart.
