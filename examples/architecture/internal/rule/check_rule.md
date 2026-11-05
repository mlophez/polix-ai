# CheckRule

Runs the checks of the deterministic rules of `ai/rules/`, so CI and the agent learn whether the code breaks a
rule that a script can decide.

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

## Logic

`CheckRule(rules []Rule, onlyChanged bool, changed []string) CheckRuleOutput` decides which checks run. It takes
what the shell read through the port and the choice of the request:

- `rules`: the rules of `ai/rules/*.yaml`, retired ones (`retired.yaml`) excluded, ordered by id.
- `onlyChanged`: whether `--changed` was given.
- `changed`: the files changed by the branch against its base, relative to the root; only with `--changed`.

`CheckRuleOutput.Run` is its decision: the rules whose check runs, in their order. The rules it follows:

1. A rule has a check when it is deterministic and names a script in `check`; the others never run.
2. Without `--changed`, every rule with a check runs.
3. With `--changed`, a rule runs when its scope matches at least one changed file; a rule without scope applies to
   the whole project and runs whenever something changed.

The domain decides the rest:

- `workflow.Bases`: the base is the branch of `--base` when given, else the base branches of polix.yaml, closest
  first.
- `rule.Result`: a check fails when its run returned an error; `Valid` requires a valid rule with a check, and
  `Tail` shows the last 15 non-empty lines of the output and the error, each cut to 500 bytes.
- `rule.Failed`: the results whose check failed.

## Response

`CheckRuleResponse`, built from the results of the checks of the output: `Ran`, how many checks ran, and
`Failed`, each failed check with its `ID`, `Check`, `Description` and `Output` (the `Tail` of the result). The
command serializes it: each failure to stderr as `FAIL <id> (<check>): <description>` followed by its output, then
the summary `<ran> checks, <failed> failed`. It exits with 0 when every check passed and with 1 otherwise.

## Port

`CheckRulePort`, declared in `check_rule_in.go` and implemented by `CheckRuleAdapter` in
`check_rule_out.go`:

- `LoadRules`: reads and parses the files of `ai/rules/`; without the folder, no rules.
- `BaseBranches`: reads the base branches of polix.yaml; without polix.yaml, the default ones.
- `Changed`: asks git for the files changed against the first base that exists, locally or on origin; when none
  exists, its error asks to set `base` in polix.yaml or to pass `--base`.
- `RunScript`: runs a check script from the root, with 5 minutes to finish; on Windows through its interpreter
  (bash of Git for Windows for `.sh`, the py launcher or python for `.py`), elsewhere through its shebang. A
  script that exits with an error, runs out of time or cannot start returns an error with its output.

## Errors

The use case stops, without running any check, when:

- `--base` is given without `--changed`.
- `--dir` is not inside a git repository.
- A file of `ai/rules/` cannot be read, is not valid yaml or holds a rule that is not `Valid` (a rule without id
  or description, or with a `check` but not `deterministic`).
- With `--changed` and without `--base`, polix.yaml cannot be read or parsed.
- With `--changed`, none of the base branches exists, or git fails to list the changed files.

## Flow

1. `CheckRuleCommand` checks the flags, turns them into a `CheckRuleRequest` and injects
   `CheckRuleAdapter{Repo: repo}` as the `CheckRulePort`.
2. The shell reads the rules and, with `--changed`, the base branches of polix.yaml (unless `--base` names one)
   and the changed files through the port.
3. `CheckRule` decides which checks run: `CheckRuleOutput`.
4. The shell carries out the output: it runs each check through the port and builds a `rule.Result` from each
   run, followed by `Valid`.
5. The shell builds the response from the results.
6. `CheckRuleCommand` serializes the response to the console and sets the exit code.
