# CheckSetup

Validates the workflow (polix.yaml, or the workflow file its argument names) and the rules (the folder `rules/` below `path`, `ai/rules/` by default) of a repository and prints every problem, so a
person or CI learns what polix would reject before a hook does.

## Trigger

```
polix config check [--dir <dir>] [file]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `file`: a workflow file to validate in place of polix.yaml, relative to the working folder, as
  `examples/polix.yaml`, with the rules, checks and base branches of the repository of `--dir`.

## Request

`Request`, filled by the command from its argument and flags:

- `Dir`, from `--dir`.
- `File`, from the argument; empty for polix.yaml of the repository.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`Decide(i setup.Inspection, file string) Output` takes what the shell observed, the part of a `setup.Inspection`
about the workflow and the rules, with a `setup.Setup` of the running version, and `File` of the request. With a
file, a missing one is the only problem, `<file> not found`; and the problems of the workflow and the warning of
the base branch name the file in place of polix.yaml: a problem that starts with it (followed by a colon or a space:
`polix.yaml:4: ...`, `polix.yaml requires ...`), and the warning where it names it as a word of its own, without
changing the error they come from. The error of a file that cannot be read already names its path, and stays. It
gathers the problems with the methods of `setup.Inspection` of `internal/app/setup/setup_problems.go`, which `polix doctor` shares:

1. `WorkflowProblems`: the error of loading polix.yaml is one problem, or each problem of a workflow that parses
   (`workflow.InvalidError`, the runners of `rules.run` included), with whether they may come from a newer polix
   (`version.Problems.NeedsNewer`, never when the workflow requires one: `workflow.TooOldError`). Among them, a verb
   the event does not allow by `workflow.EventVerbs`, the verbs both agents support on it (`run`, `inject` and
   `notify` on `start`, `subagent_start`, `post_edit`, `post_command` and `post_commit`; `run`, `inject`, `block` and `notify` on
   `prompt`, `pre_edit`, `pre_command` and `pre_commit`; `run`, `block` and `notify` on `stop` and
   `subagent_stop`), named with the event, the verb, why and the verbs the event allows. An entry that never runs,
   after a `block` without `when`, `unless` or `once` of the same event other than `stop` and `subagent_stop`:
   `never runs: the entry of line M always blocks the event`, at the line of the later entry, a shortcut named by
   its path. And a notice a later gate could swallow, on every event: a `notify` followed in its event by an entry
   that may block it, a `block` or a `run` on an event outside `workflow.Informed`, as `the notice is not shown when
   the entry of line M blocks the event: move it after that entry or into its message`, at the line of the
   `notify`, since a blocked event shows the user no message.
2. The error of reading the rules is one problem; else `RuleProblems`: the problems of the loader
   (`rule.Loaded.Problems`): every invalid rule, in the words `<file> <id>: no description`, then every id repeated,
   one an earlier rule or a rule of `retired.yaml` already has, as `<file> <id>: id repeated, also in <other file>`;
   and every check that does not exist.
3. With problems, the output holds them and, when they may come from a newer polix, the hint `NewerHint`.
4. Without problems, `BaseWarning`: when a deterministic rule has a check and no base branch exists,
   `polix rules check --changed` would fail.

## Response

`Response` is the output: `File` (the file as the console names it: the argument, or polix.yaml), `Problems`,
`Hint`, `Warning`, `Found` (whether the workflow file exists), `Rules` (how many rules) and `RulesPath` (the folder they were read from, `Workflow.RulesPath`). The command prints each
problem to stderr and the hint as a warning, and exits with 1; else it prints the warning to stderr and
`ok: <file> and <n> rules of <rules path>/` (or `ok: no polix.yaml
(only the built-in protections apply) and ...`) to stdout, and exits with 0.

## Port

`Port`, implemented by `Adapter` on the repository:

- `LoadWorkflow`: loads the file of the field `Path` of the adapter (polix.yaml at the root, or the file of the
  argument) through the field `Workflow` of the adapter, the loader of `LoadWorkflow`
  (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`; without it, the default
  workflow and false; a workflow that parses with problems comes back with its `workflow.InvalidError`.
- `LoadRules`: loads the rules of the folder it is given (`Workflow.RulesPath`, `rules/` below `path` of the workflow, `ai/rules/` by default)
  through the field `Rules` of the adapter, the loader of `LoadRule` (`internal/app/rule/load`) that `cmd/polix`
  builds once and passes to `Command`: every rule but those of `retired.yaml`, ordered by id, with the problems of
  the invalid ones and of the ids repeated; without the folder, none; a file that cannot be read or parsed is an
  error.
- `Missing`: whether the file of a check does not exist below the root.
- `ResolveBase`: why no base branch of the workflow exists, locally or on origin.

## Errors

- Outside a repository the command fails with the error of git and exits with 1.
- An empty file argument fails, and exits with 1.
- The errors of reading polix.yaml and the rules are problems of the output, never errors of the shell.
- Without the version of the running polix there is no setup to inspect: the shell fails with the error of
  `setup.Setup.Valid`. The command always gives it (`build.Current`).

## Flow

1. `Command` opens the repository of `--dir` and injects `Adapter{Repo: repo, Path: path, Rules: rules, Workflow:
   loadWorkflow}`, with the file of the argument or polix.yaml of the root, and the loaders of the rules and of
   polix.yaml it was given.
2. The shell reads the workflow, then the rules of its folder of rules, below its `path`, whether each check exists and whether a base
   branch exists, and builds the `setup.Inspection` of it, validated.
3. `Decide` decides the problems, the hint and the warning.
4. The shell builds the response, and the command prints it and sets the exit code.
