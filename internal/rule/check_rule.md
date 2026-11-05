# CheckRule

Runs the checks of the deterministic rules of `ai/rules/`, so CI and the agent learn whether the code breaks a
rule that a script or a test can decide.

## Trigger

```
polix rules check [--dir <dir>] [--changed] [--base <branch>]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `--changed`: only the rules whose scope matches the files changed by the branch.
- `--base`: the branch the change merges into, locally or on origin, for `--changed`; without it, the closest of
  `base` in polix.yaml, else `main`. Without `--changed` it is an error.

## Request

`CheckRuleRequest`, filled by the command from its flags:

- `Dir`, `OnlyChanged` and `Base`, from `--dir`, `--changed` and `--base`. `Base` needs `OnlyChanged`, which the
  command checks before running.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`CheckRule(rules []Rule, wf workflow.Workflow, onlyChanged bool, changed []string) CheckRuleOutput` decides which
checks run and how. It takes what the shell read through the port and the choice of the request:

- `rules`: the rules of `ai/rules/*.yaml`, retired ones (`retired.yaml`) excluded, ordered by id.
- `wf`: the workflow of polix.yaml, the default one without the file; only its runners (`rules.run`) matter here.
- `onlyChanged`: whether `--changed` was given.
- `changed`: the files changed by the branch against its base, relative to the root; only with `--changed`.

`CheckRuleOutput.Run` is its decision: a `rule.Run` for each check that runs, in the order of the rules, with the
argv that runs it. The rules it follows:

1. A rule has a check when it is deterministic and names a file in `check` (a script, a test, any file); the
   others never run.
2. Without `--changed`, every rule with a check runs.
3. With `--changed`, a rule runs when its scope matches at least one changed file; a rule without scope applies to
   the whole project and runs whenever something changed.
4. The argv of a check is that of the first runner of `rules.run` whose glob matches the base name of the check,
   with `{file}`, `{dir}`, `{id}` and `{name}` expanded (`Workflow.CheckCommand`); when no runner matches it is
   empty, and the check runs as a program.

The domain decides the rest:

- `workflow.Bases`: the base is the branch of `--base` when given, else the base branches of polix.yaml, closest
  first.
- `rule.Run`: `Valid` requires a valid rule with a check.
- `rule.Result`: a check fails when its run returned an error; `Valid` requires a valid rule with a check, and
  `Tail` shows the last 15 non-empty lines of the output and the error, each cut to 500 bytes.
- `rule.Failed`: the results whose check failed.

## Response

`CheckRuleResponse`, built from the results of the checks of the output: `Ran`, how many checks ran, and
`Failed`, each failed check (`CheckRuleFailed`) with its `ID`, `Check`, `Description` and `Output` (the `Tail` of
the result). The command serializes it: each failure to stderr as `FAIL <id> (<check>): <description>` followed by
its output, then the summary `<ran> checks, <failed> failed` to stdout. It exits with 0 when every check passed
and with 1 otherwise.

## Port

`CheckRulePort`, declared in `check_rule_in.go` and implemented by `CheckRuleAdapter` in
`check_rule_out.go`:

- `LoadRules`: reads and parses the files of `ai/rules/`, `retired.yaml` left out, ordered by id; without the
  folder, no rules.
- `LoadWorkflow`: reads polix.yaml, always, for its runners and its base branches; without polix.yaml, the default
  workflow. Of the problems that `polix check` reports on a workflow that parses, only those of `rules`
  stop the use case (see Errors); the rest are left to `polix check`.
- `Changed`: asks git for the files changed against the first base that exists, locally or on origin; when none
  exists, its error asks to set `base` in polix.yaml or to pass `--base`.
- `RunCheck`: runs a check from the root, with 5 minutes to finish: the argv of its runner or, when empty, the file as
  a program, on Windows through its interpreter (bash of Git for Windows for `.sh`, the py launcher or python for
  `.py`), elsewhere through its shebang. On Windows a runner whose program is `bash`, `sh`, `python`, `python3` or
  `py` runs the program of that name the PATH finds (an active virtualenv wins), unless it is an alias of the Store
  or, for `bash` and `sh`, the bash of WSL in the Windows folder; then the interpreter of its kind (`proc.Command`),
  with the arguments of the runner kept. A check that exits with an error, runs out of time or cannot start returns an
  error with its output.

## Errors

The use case stops, without running any check, when:

- `--base` is given without `--changed`.
- `--dir` is not inside a git repository.
- A file of `ai/rules/` cannot be read, is not valid yaml or holds a rule that is not `Valid` (a rule without id
  or description, with a `check` but not `deterministic`, or with a `check` given as an absolute path).
- polix.yaml cannot be read, is not valid yaml, has an unknown top level key or requires a newer polix.
- The key `rules` of polix.yaml is not valid (`Rules.Valid`): a runner with an empty argv, an invalid glob or an
  unknown placeholder, which would otherwise change how a check runs.
- With `--changed`, none of the base branches exists, or git fails to list the changed files.

The command returns the error, which `pkg/cli.Run` prints in one line, `polix: <error>`, and exits with 1.

## Flow

1. `CheckRuleCommand` checks the flags, turns them into a `CheckRuleRequest` and injects
   `CheckRuleAdapter{Repo: repo}` as the `CheckRulePort`.
2. The shell reads the rules, stopping on one that is not `Valid`, and the workflow, stopping on invalid
   runners, through the port and, with `--changed`, the changed files against
   `--base` or else the base branches of the workflow.
3. `CheckRule` decides which checks run and with which argv: `CheckRuleOutput`.
4. The shell carries out the output: it runs each check through the port and builds a `rule.Result` from each
   run, followed by `Valid`.
5. The shell builds the response from the results.
6. `CheckRuleCommand` serializes the response to the console and sets the exit code.
