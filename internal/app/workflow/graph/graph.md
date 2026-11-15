# GraphWorkflow

Prints the workflow of polix.yaml as a Mermaid flowchart, so a person sees at a glance what polix does on each
event.

## Trigger

```
polix graph [--dir <dir>]
```

- `--dir`: any folder of the repository; defaults to the current one.

## Request

`Request`, filled by the command from its flags:

- `Dir`, from `--dir`.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`Decide(wf workflow.Workflow, found bool) Output` draws the workflow. It takes what the shell read through the
port:

- `wf`: the workflow of polix.yaml, drawn as it is even when `polix config check` would report problems, with its
  protected files and forbidden commands (`ProtectedFiles`, `ForbiddenCommands`) and the other shortcuts of
  `guardrails` already expanded into entries of `on` and of `githooks` (`Githooks`).
- `found`: whether polix.yaml exists.

`Output` holds `Mermaid`, the flowchart, and `Missing`, set when there is no polix.yaml. The rules it follows:

1. Without polix.yaml there is nothing to draw: `Missing`, and no flowchart.
2. The flowchart starts with `flowchart TD`. With protected files or forbidden commands, a subgraph `guardrails`
   comes first, with the protections the hook applies before the entries of every event: first one node per item of
   `forbidden_commands` the presets of `guardrails.builtin` add (`workflow.ForbiddenCommand.From` set), then one node
   `protected_files: <globs>`, the globs joined with `, `, then one node per forbidden command written in the file,
   each forbidden command as `forbidden_commands: <pattern>`, followed by `<br/>` and its reason when it has one and,
   for a preset item, by `<br/>from guardrails.builtin.<key>`, linked with `-->`. Then one subgraph per event
   with entries, in the order of `workflow.Events`, and then one subgraph `githooks_<event> [githooks.<event>]` per
   event of `githooks` with entries, in the order of `workflow.GithookEvents` (`pre_commit`, `commit_msg`), so the gates
   git runs are told apart from the events of the AI of the same name. Each subgraph is closed by `end`.
3. Each entry is a node `n<k>`, numbered across the whole flowchart after the nodes of `guardrails`, linked to the
   previous entry of its event with `-->`, in the order of the workflow: those of the shortcuts before those written
   in `on`, and on `githooks.pre_commit` those of `builtin.check_polix_files`, `check_on_save` and the items of
   `check_on_stop` with `also_on_save` before those written in `githooks`.
4. The label of a node of an event joins with `<br/>`: its `when` and `unless` conditions, as `when key=value` and
   `unless key=value` ordered by key; the verb of its action: `block: <message>`, followed by `(once)` with
   `once: true`; `run` with its command or `rules <target>`, `inject` with `file <file>`, `rules <target>` or
   `: <text>`, and `notify: <message>`, each followed by `(always)` with `always: true`; and `from <shortcut>` when the entry was expanded
   from a shortcut (`From`, as `guardrails.format_on_edit`), nothing for an entry written in `on`. In every label,
   double quotes become single ones, and the line breaks of a message become `<br/>`, so each statement keeps one
   line.

## Response

`Response`, from the output: `Mermaid`, the flowchart, and `Missing`, when there is no polix.yaml. The command
prints the flowchart to stdout and exits with 0; without polix.yaml it fails with
`polix.yaml not found` and exits with 1.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `LoadWorkflow`: loads polix.yaml through the field `Workflow` of the adapter, the loader of `LoadWorkflow`
  (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`, and tells whether it exists;
  without it, the default workflow. The problems of a workflow that parses (`workflow.InvalidError`) are not an error:
  the workflow is drawn as it is.

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--dir` is not inside a git repository.
- polix.yaml cannot be read, is not valid yaml, does not decode into the schema of polix.yaml (a malformed
  `rules.run` among them), has an unknown top level key or requires a newer polix.
- There is no polix.yaml.

## Flow

1. `Command` turns the flags into a `Request` and injects `Adapter{Repo: repo, Workflow: loadWorkflow}`, with the
   loader of polix.yaml it was given, as the `Port`.
2. The shell loads the workflow through the port.
3. `Decide` draws it: `Output`.
4. The shell builds the response from the output.
5. `Command` fails without polix.yaml, else prints the flowchart.
