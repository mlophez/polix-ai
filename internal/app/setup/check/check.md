# CheckSetup

Validates the workflow (polix.yaml) and the rules (the folder `rules.path`, `ai/rules/` by default) of a repository and prints every problem, so a
person or CI learns what polix would reject before a hook does.

## Trigger

```
polix config check [--dir <dir>]
```

- `--dir`: any folder of the repository; defaults to the current one.

## Request

`Request`, filled by the command from its flags:

- `Dir`, from `--dir`.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`Decide(i setup.Inspection) Output` takes what the shell observed, the part of a `setup.Inspection` about the
workflow and the rules, with a `setup.Setup` of the running version, and gathers the problems with the methods of
`setup.Inspection` of `internal/app/setup/setup_problems.go`, which `polix doctor` shares:

1. `WorkflowProblems`: the error of loading polix.yaml is one problem, or each problem of a workflow that parses
   (`workflow.InvalidError`, the runners of `rules.run` included), with whether they may come from a newer polix
   (`version.Problems.NeedsNewer`, never when the workflow requires one: `workflow.TooOldError`).
2. The error of reading the rules is one problem; else `RuleProblems`: every invalid rule (`rule.Loaded.Problems`,
   in the words `<file> <id>: no description`), every id repeated (`rule.Rules.Repeated`) and every check that does
   not exist.
3. With problems, the output holds them and, when they may come from a newer polix, the hint `NewerHint`.
4. Without problems, `BaseWarning`: when a deterministic rule has a check and no base branch exists,
   `polix rules check --changed` would fail.

## Response

`Response` is the output: `Problems`, `Hint`, `Warning`, `Found` (whether polix.yaml exists), `Rules`
(how many rules) and `RulesPath` (the folder they were read from, `Workflow.RulesPath`). The command prints each
problem to stderr and the hint as a warning, and exits with 1; else it prints the warning to stderr and
`ok: polix.yaml and <n> rules of <rules path>/` (or `ok: no polix.yaml
(only the built-in protections apply) and ...`) to stdout, and exits with 0.

## Port

`Port`, implemented by `Adapter` on the repository:

- `LoadWorkflow`: loads polix.yaml through the field `Workflow` of the adapter, the loader of `LoadWorkflow`
  (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`; without it, the default
  workflow and false; a workflow that parses with problems comes back with its `workflow.InvalidError`.
- `LoadRules`: loads the rules of the folder it is given (`rules.path` of the workflow, `ai/rules/` by default)
  through the field `Rules` of the adapter, the loader of `LoadRule` (`internal/app/rule/load`) that `cmd/polix`
  builds once and passes to `Command`: every rule but those of `retired.yaml`, ordered by id, with the problems of
  the invalid ones; without the folder, none; a file that cannot be read or parsed is an error.
- `Missing`: whether the file of a check does not exist below the root.
- `ResolveBase`: why no base branch of the workflow exists, locally or on origin.

## Errors

- Outside a repository the command fails with the error of git and exits with 1.
- The errors of reading polix.yaml and the rules are problems of the output, never errors of the shell.
- Without the version of the running polix there is no setup to inspect: the shell fails with the error of
  `setup.Setup.Valid`. The command always gives it (`build.Current`).

## Flow

1. `Command` opens the repository of `--dir` and injects `Adapter{Repo: repo, Rules: rules, Workflow:
   loadWorkflow}`, with the loaders of the rules and of polix.yaml it was given.
2. The shell reads the workflow, then the rules of its `rules.path`, whether each check exists and whether a base
   branch exists, and builds the `setup.Inspection` of it, validated.
3. `Decide` decides the problems, the hint and the warning.
4. The shell builds the response, and the command prints it and sets the exit code.
