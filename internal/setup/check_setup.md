# CheckSetup

Validates the workflow (polix.yaml) and the rules (the folder `rules.path`, `ai/rules/` by default) of a repository and prints every problem, so a
person or CI learns what polix would reject before a hook does.

## Trigger

```
polix check [--dir <dir>]
```

- `--dir`: any folder of the repository; defaults to the current one.

## Request

`CheckSetupRequest`, filled by the command from its flags:

- `Dir`, from `--dir`.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`CheckSetup(running string, found bool, wfErr error, rulesPath string, rules []rule.Rule, invalid []string,
rulesErr error, missing map[string]bool, baseErr error) CheckSetupOutput` gathers the problems, with the domain of
`setup_problems.go`, which `polix doctor` shares:

1. `WorkflowProblems`: the error of loading polix.yaml is one problem, or each problem of a workflow that parses
   (`workflow.Invalid`, the runners of `rules.run` included), with whether they may come from a newer polix
   (`version.NeedsNewer`, never when the workflow requires one: `workflow.TooOld`).
2. The error of reading the rules is one problem; else `RuleProblems`: every invalid rule (`rule.ParseAll`, in the
   words `<file> <id>: no description`), every id repeated (`rule.Repeated`) and every check that does not exist.
3. With problems, the output holds them and, when they may come from a newer polix, the hint `NewerHint`.
4. Without problems, `BaseWarning`: when a deterministic rule has a check and no base branch exists,
   `polix rules check --changed` would fail.

## Response

`CheckSetupResponse` is the output: `Problems`, `Hint`, `Warning`, `Found` (whether polix.yaml exists), `Rules`
(how many rules) and `RulesPath` (the folder they were read from, `Workflow.RulesPath`). The command prints each
problem to stderr and the hint as a warning, and exits with 1; else it prints the warning to stderr and
`ok: polix.yaml and <n> rules of <rules path>/` (or `ok: no polix.yaml
(only the built-in protections apply) and ...`) to stdout, and exits with 0.

## Port

`CheckSetupPort`, implemented by `CheckSetupAdapter` on the repository:

- `LoadWorkflow`: reads polix.yaml; without it, the default workflow and false; a workflow that parses with
  problems comes back with its `workflow.Invalid`.
- `LoadRules`: reads the `*.yaml` of the folder it is given (`rules.path` of the workflow, `ai/rules/` by
  default) but `retired.yaml` with `rule.ParseAll`, ordered by id; without the folder, none; a file that cannot be
  read or parsed is an error.
- `Missing`: whether the file of a check does not exist below the root.
- `ResolveBase`: why no base branch of the workflow exists, locally or on origin.

## Errors

- Outside a repository the command fails with the error of git and exits with 1.
- The errors of reading polix.yaml and the rules are problems of the output, never errors of the shell.

## Flow

1. `CheckSetupCommand` opens the repository of `--dir` and injects `CheckSetupAdapter{Repo: repo}`.
2. The shell reads the workflow, then the rules of its `rules.path`, whether each check exists and whether a base
   branch exists.
3. `CheckSetup` decides the problems, the hint and the warning.
4. The shell builds the response, and the command prints it and sets the exit code.
