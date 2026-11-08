# CheckRule

Runs the checks of the deterministic rules of the folder `rules.path` of polix.yaml (`ai/rules/` by default), so CI and the agent learn whether the code breaks a
rule that a script or a test can decide.

## Trigger

```
polix rules check [--dir <dir>] [--changed] [--base <branch>]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `--changed`: only the rules whose scope matches the files changed by the branch.
- `--base`: the branch the change merges into, locally or on origin, for `--changed`; without it, the closest of
  `base_branch` in polix.yaml, else `main`. Without `--changed` it is an error.

## Request

`CheckRuleRequest`, filled by the command from its flags:

- `Dir`, `OnlyChanged` and `Base`, from `--dir`, `--changed` and `--base`. `Base` needs `OnlyChanged`, which the
  command checks before running.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`CheckRule(rules []Rule, wf workflow.Workflow, onlyChanged bool, changed []string, outcomes []Outcome)
CheckRuleOutput` decides which checks run and how, in the step shape. It takes what the shell read through the
port, the choice of the request and what the runs carried out so far gave:

- `rules`: the rules of the `*.yaml` of the folder of rules, retired ones (`retired.yaml`) excluded, ordered by id.
- `wf`: the workflow of polix.yaml, the default one without the file; only its runners (`rules.run`) matter here
  (the shell uses its `rules.path` to read the rules).
- `onlyChanged`: whether `--changed` was given.
- `changed`: the files changed by the branch against its base, relative to the root; only with `--changed`.
- `outcomes`: the `rule.Outcome` of each run carried out so far, in order.

It plans the runs of the rules (`rule.Plan`) and walks them, and the reruns of each failed shared run, taking one
outcome for each. `CheckRuleOutput.Next` is the first run without outcome, the next one the shell carries out;
when none is left it is nil and `CheckRuleOutput.Results` holds a `rule.Result` for every rule that ran, in the
order of the rules. The rules it follows:

1. A rule has a check when it is deterministic and names a file in `check` (a script, a test, any file); the
   others never run.
2. Without `--changed`, every rule with a check runs.
3. With `--changed`, a rule runs when its scope matches at least one changed file; a rule without scope applies to
   the whole project and runs whenever something changed.
4. A check goes to the first runner of `rules.run` whose glob matches it: without `/` its base name, with `/` its
   path relative to the root (`pkg/glob`). When no runner matches, the check runs alone as a program.
5. The checks of one runner whose argv with `{file}`, `{dir}`, `{id}` and `{name}` expanded is the same share one
   run, where `{names}` (the names joined with `|`), `{files}` and `{dirs}` cover them all
   (`Workflow.CheckCommands`). Runs come in the order of their first rule.
6. A run that passes makes every rule of it pass, with its output. A run of one rule that fails makes it fail.
7. A shared run that fails is rerun once per rule, with the argv of that rule alone (`Workflow.CheckCommand`),
   and each rerun decides its rule; when that argv is the shared one for every rule (a runner without
   placeholders), every rule fails with the shared output and nothing is rerun.

The domain decides the rest:

- `workflow.Bases`: the base is the branch of `--base` when given, else the base branches of polix.yaml, closest
  first.
- `rule.Plan`: turns the rules with a check into runs through `Workflow.CheckCommands`.
- `rule.Run`: `Valid` requires at least one rule, each valid and with a check, and an argv when there are several;
  `Settle` turns its `rule.Outcome` into the results of its rules or into the reruns of rule 7.
- `rule.Outcome`: the output and the error of carrying out a run.
- `rule.Result`: a check fails when its run returned an error; `Valid` requires a valid rule with a check, and
  `Tail` shows the last 15 non-empty lines of the output and the error, each cut to 500 bytes.
- `rule.Failed`: the results whose check failed.

## Response

`CheckRuleResponse`, built from the results of the output: `Ran`, how many rules were decided, and
`Failed`, each failed check (`CheckRuleFailed`) with its `ID`, `Check`, `Description` and `Output` (the `Tail` of
the result). The command serializes it: each failure to stderr as `FAIL <id> (<check>): <description>` followed by
its output, then the summary `<ran> checks, <failed> failed` to stdout. It exits with 0 when every check passed
and with 1 otherwise.

## Port

`CheckRulePort`, declared in `check_rule_in.go` and implemented by `CheckRuleAdapter` in
`check_rule_out.go`:

- `LoadRules`: reads and parses the files of the folder it is given (`Workflow.RulesPath`), `retired.yaml` left
  out, ordered by id; without the folder, no rules.
- `LoadWorkflow`: reads polix.yaml, always, for its runners and its base branches; without polix.yaml, the default
  workflow. Of the problems that `polix check` reports on a workflow that parses, only those of `rules`
  stop the use case (see Errors); the rest are left to `polix check`.
- `Changed`: asks git for the files changed against the first base that exists, locally or on origin; when none
  exists, its error asks to set `base_branch` in polix.yaml or to pass `--base`.
- `RunCheck`: runs one invocation from the root, which may decide several rules, with 5 minutes to finish: the argv
  of its runner or, when empty, the check of its only rule as a program, on Windows through its interpreter (bash of Git for Windows for `.sh`, the py launcher or python for
  `.py`), elsewhere through its shebang. On Windows a runner whose program is `bash`, `sh`, `python`, `python3` or
  `py` runs the program of that name the PATH finds (an active virtualenv wins), unless it is an alias of the Store
  or, for `bash` and `sh`, the bash of WSL in the Windows folder; then the interpreter of its kind (`proc.Command`),
  with the arguments of the runner kept. A check that exits with an error, runs out of time or cannot start returns an
  error with its output. The command fills the field `Log` of the adapter with its stderr, so each invocation is
  reported as it runs: `run <command> (<ids>)` before, with the count and the first 5 ids for more than 5 rules,
  and `ok in <duration>` or `fail in <duration>` after.

## Errors

The use case stops, without running any check, when:

- `--base` is given without `--changed`.
- `--dir` is not inside a git repository.
- A file of the folder of rules cannot be read, is not valid yaml or holds a rule that is not `Valid` (a rule without id
  or description, with a `check` but not `deterministic`, or with a `check` given as an absolute path).
- polix.yaml cannot be read, is not valid yaml, has an unknown top level key or requires a newer polix.
- The key `rules` of polix.yaml is not valid (`Rules.Valid`): a `path` that is not a folder below the root, a
  runner with an empty argv or glob, an unknown placeholder, or `{files}` or `{dirs}` inside a longer argument,
  which would otherwise change how a check runs.
- A `rule.Outcome` or a `rule.Result` is not `Valid`.
- With `--changed`, none of the base branches exists, or git fails to list the changed files.

The command returns the error, which `pkg/cli.Run` prints in one line, `polix: <error>`, and exits with 1.

## Flow

1. `CheckRuleCommand` checks the flags, turns them into a `CheckRuleRequest` and injects
   `CheckRuleAdapter{Repo: repo}` as the `CheckRulePort`.
2. The shell reads the workflow, stopping on an invalid key `rules`, and then the rules of its `rules.path`,
   stopping on one that is not `Valid`, through the port and, with `--changed`, the changed files against
   `--base` or else the base branches of the workflow.
3. The shell calls `CheckRule` in a loop, from one call site, with the outcomes so far: while
   `CheckRuleOutput.Next` is set, it runs it through `RunCheck`, builds a `rule.Outcome` from the run, followed by
   `Valid`, and appends it.
4. When `Next` is nil, the shell checks each of `CheckRuleOutput.Results` with `Valid`.
5. The shell builds the response from the results.
6. `CheckRuleCommand` serializes the response to the console and sets the exit code.
