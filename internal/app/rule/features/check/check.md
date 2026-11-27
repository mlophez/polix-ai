# CheckRule

Validates the rules of the folder `rules/` below `path` of polix.yaml (`ai/rules/` by default) and their format, as
`polix rules fmt --check` does, and with `--run` runs the checks of the deterministic rules too, so CI and the agent
learn whether a rule is broken or not formatted and whether the code breaks a rule that a script or a test can decide.

## Trigger

```
polix rules check [--dir <dir>] [--run] [--changed] [--base <branch>]
```

- `--run`: once the rules are valid and formatted, run their checks too; without it no check runs.
- `--dir`: any folder of the repository; defaults to the current one.
- `--changed`: with `--run`, only the rules whose scope matches the files changed by the branch.
- `--base`: the branch the change merges into, locally or on origin, for `--changed`; without it, the closest of
  `base_branch` in polix.yaml, else `main`. Without `--changed` it is an error, as `--changed` is without `--run`.

## Request

`Request`, filled by the command from its flags:

- `Dir`, `Run`, `OnlyChanged` and `Base`, from `--dir`, `--run`, `--changed` and `--base`. `OnlyChanged` needs
  `Run` and `Base` needs `OnlyChanged`, which the command checks before running.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`Decide(folder rule.Folder, wf workflow.Workflow, sel rule.Selection, outcomes []rule.Outcome) Output` decides whether
the rules are formatted and which checks run and how, in the step shape. It takes what the shell read through the port,
the choice of the request and what the runs carried out so far gave:

- `folder`: the `rule.Folder`, valid, of the folder of rules: its `Dir`, its `Rules`, those of its `*.yaml` ordered by
  id, and its `Files`, the data of each `*.yaml` by name as a `rule.Source`.
- `wf`: the workflow of polix.yaml, the default one without the file; only its runners (`rules.run`) matter here
  (the shell uses its `Workflow.RulesPath`, `rules/` below `path`, to read the rules).
- `sel`: the `rule.Selection` the shell builds and validates: `Run`, whether `--run` was given, `OnlyChanged`,
  whether `--changed` was given, and
  `Changed`, the files changed by the branch against its base, relative to the root, only with `--changed`.
  `Selection.Narrow` keeps the rules it asks for.
- `outcomes`: the `rule.Outcome` of each run carried out so far, in order.

First the format: each file that `rule.Source.Format` would change or cannot format, as `<dir>/<name>`
(`rule.Folder.Unformatted`), goes to `Output.Unformatted`, with `Output.Fix`, `polix rules fmt` (`rule.FixCommand`),
and each file it cannot format to `Output.Problems`, as `<dir>/<name>: <error>`; with any of them, or
without `Run`, it returns without runs. Else `Output.Ran` is true, and it plans the runs of the rules (`rule.Rules.Plan`) and walks them, and the reruns of each failed shared run, taking one
outcome for each. `Output.Next` is the first run without outcome, the next one the shell carries out;
when none is left it is nil and `Output.Results` holds a `rule.Result` for every rule that ran, in the
order of the rules. The rules it follows:

1. A rule has a check when it is deterministic and names a file in `check` (a script, a test, any file); the
   others never run.
2. Without `--changed`, every rule with a check runs.
3. With `--changed`, a rule runs when its scope matches at least one changed file; a rule without scope applies to
   the whole project and runs whenever something changed.
4. A check goes to the first runner of `rules.run` whose glob matches it: without `/` its base name, with `/` its
   path relative to the root (`internal/platform/glob`). When no runner matches, the check runs alone as a
   program.
5. The checks of one runner whose argv with `{file}`, `{dir}`, `{id}` and `{name}` expanded is the same share one
   run, where `{names}` (the names joined with `|`), `{files}` and `{dirs}` cover them all
   (`Workflow.CheckCommands`). Runs come in the order of their first rule.
6. A run that passes makes every rule of it pass, with its output. A run of one rule that fails makes it fail.
7. A shared run that fails is rerun once per rule, with the argv of that rule alone (`Workflow.CheckCommand`),
   and each rerun decides its rule; when that argv is the shared one for every rule (a runner without
   placeholders), every rule fails with the shared output and nothing is rerun.

The domain decides the rest:

- `Workflow.Bases`: the base is the branch of `--base` when given, else the base branches of polix.yaml, closest
  first.
- `rule.Folder`: `Valid` requires its `Dir` and every rule valid; `Unformatted` formats each file as `polix rules fmt`
  does (`rule.Source.Format`), splitting the files it would change from those it cannot format, with their error.
- `rule.Selection`: `Valid` requires `Run` for `OnlyChanged`, and `OnlyChanged` for changed files.
- `rule.Rules`: `Matching` keeps the rules whose scope matches the changed files, and `Plan` turns the rules with a
  check into runs through `Workflow.CheckCommands`.
- `rule.Run`: `Valid` requires at least one rule, each valid and with a check, and an argv when there are several;
  `Settle` turns its `rule.Outcome` into the results of its rules or into the reruns of rule 7.
- `rule.Outcome`: the output and the error of carrying out a run.
- `rule.Result`: a check fails when its run returned an error; `Valid` requires a valid rule with a check, and
  `Tail` shows the last 15 non-empty lines of the output and the error, each cut to 500 bytes.
- `rule.Results.Failed`: the results whose check failed.

## Response

`Response`, built from the output and its results: `Rules`, how many rules are valid; `Unformatted`, `Problems` and `Fix`, of the
output; `Ran`, whether the checks ran; `Checks`, how many rules were decided; and `Failed`, each failed check (a
`Failed`) with its `ID`, `Check`, `Description` and `Output` (the `Tail` of the result). The command serializes it:

- With files not formatted, each problem to stderr as it is and each other file as
  `<file> is not formatted: run <fix>`, then
  `<rules> rules valid, <n> files not formatted` to stdout, and exits with 1.
- Without `--run`, `<rules> rules valid and formatted` to stdout, and exits with 0.
- Else each failure to stderr as `FAIL <id> (<check>): <description>` followed by its output, then the summary
  `<checks> checks, <failed> failed` to stdout. It exits with 0 when every check passed and with 1 otherwise.

## Port

`Port`, declared in `check_in.go` and implemented by `Adapter` in `check_out.go`:

- `LoadRules`: loads the rules of the folder it is given (`Workflow.RulesPath`) through the field `Rules` of the
  adapter, the loader `ruleadapters.Folder.Load` (`internal/app/rule/adapters`) that `cmd/polix` builds once and passes to
  `Command`: ordered by id; without the folder, no rules. It stops where a strict reading
  does (`rule.Loaded.Strict`): at a `retired.yaml` of the folder, else at the first invalid rule of the files in the order of their names, with
  its file, else at the first id repeated, else at a file that cannot be read or is not yaml.
- `LoadWorkflow`: loads polix.yaml, always, for its runners and its base branches, through the field `Workflow` of the
  adapter, the loader `workflowadapters.File.Load` (`internal/app/workflow/adapters`) that `cmd/polix` builds once and passes to
  `Command`; without polix.yaml, the default workflow. Of the problems that `polix config check` reports on a workflow
  that parses, only those of `rules` stop the use case (see Errors); the rest are left to `polix config check`.
- `ReadRuleFiles`: reads the `*.yaml` of the folder through `internal/platform/file`, by name, subfolders left out;
  a missing folder has no files and no error.
- `Changed`: asks git for the files changed against the first base that exists, locally or on origin; when none
  exists, its error asks to set `base_branch` in polix.yaml or to pass `--base`.
- `RunCheck`: runs one invocation from the root, which may decide several rules, with 5 minutes to finish: the argv of
  its runner or, when empty, the check of its only rule as a program, on Windows through its interpreter (bash of Git
  for Windows for `.sh`, the py launcher or python for `.py`), elsewhere through its shebang. On Windows a runner whose
  program is `bash`, `sh`, `python`, `python3` or `py` runs the program of that name the PATH finds (an active
  virtualenv wins), unless it is an alias of the Store or, for `bash` and `sh`, the bash of WSL in the Windows folder;
  then the interpreter of its kind (`proc.Command`), with the arguments of the runner kept. A check that exits with an
  error, runs out of time or cannot start returns an error with its output. The command fills the field `Log` of the
  adapter with its stderr, so each invocation is reported as it runs: `run <command> (<ids>)` before, with the count and
  the first 5 ids for more than 5 rules, and `ok in <duration>` or `fail in <duration>` after.

## Errors

The use case stops, without running any check, when:

- `--changed` is given without `--run`: `--changed needs --run`.
- `--base` is given without `--changed`.
- `--dir` is not inside a git repository.
- A file of the folder of rules cannot be read, is not valid yaml or holds a rule that is not `Valid` (a rule without id
  or description, with a `check` but not `deterministic`, or with a `check` given as an absolute path).
- The folder holds a `retired.yaml`: the error is `<path> is no longer read: delete it, its ids are free`, before any
  invalid rule.
- Two rules share an id: the error is the problem of the loader,
  `<file> <id>: id repeated, also in <other file>`, for the first such rule in the order of the files; an invalid
  rule of any file wins over it.
- polix.yaml cannot be read, is not valid yaml, has an unknown top level key or requires a newer polix.
- The key `rules` of polix.yaml is not valid (`Rules.Valid`): a `path` that is not a folder below the root, a
  runner with an empty argv or glob, an unknown placeholder, or `{files}` or `{dirs}` inside a longer argument,
  which would otherwise change how a check runs.
- A `rule.Outcome` or a `rule.Result` is not `Valid`.
- With `--changed`, none of the base branches exists, or git fails to list the changed files.

The command returns the error, which `internal/platform/cli.Run` prints in one line, `polix: <error>`, and exits
with 1.

## Flow

1. `Command` checks the flags, turns them into a `Request` and injects
   `Adapter{Repo: repo, Log: stderr, Rules: rules, Workflow: loadWorkflow}`, with the loaders of the rules and of
   polix.yaml it was given, as the `Port`.
2. The shell reads, in `load`, the workflow, stopping on an invalid key `rules`, and then the rules of its folder of
   rules, below its `path`, stopping on one that is not `Valid`, through the port and, with `--changed`, the changed
   files against `--base` or else the base branches of the workflow; then the files of the folder of rules, which
   with the rules make a `rule.Folder`, followed by `Valid`.
3. The shell calls `Decide` in a loop, from one call site, with the outcomes so far: while
   `Output.Next` is set (never without `--run` nor with a file not formatted), it runs it through `RunCheck`, builds a `rule.Outcome` from the run, followed by
   `Valid`, and appends it.
4. When `Next` is nil, `response` checks each of `Output.Results` with `Valid`.
5. `response` builds the response from the results.
6. `Command` serializes the response to the console and sets the exit code.
