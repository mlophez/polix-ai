# DiagnoseSetup

Checks that the machine and the repository are ready for polix: the binary and its version, the PATH the hooks
use, git, the workflow, the rules and their checks, the hooks of Claude Code, the plugin, Codex only when polix is
registered for it (Codex support is not implemented and untested) and the state.

## Trigger

```
polix doctor [--dir <dir>] [--hook] [--offline]
```

- `--dir`: the repository to check; defaults to the current folder.
- `--hook`: for the session start hook: no network, Claude Code not asked, only the problems, never fails.
- `--offline`: do not ask for the latest release; the one of the last check is used.

## Request

`Request`: `Dir`, `Hook`, `Offline`, `Version` (`build.Current`) and `Now`, recorded in the cache.

## Logic

`Decide(i setup.Inspection, hook bool) Output` turns the `setup.Inspection`, whose files are already decoded,
into findings (`setup.Finding`: level, area, message, each built by `found`), in this order: the binary (version
against the latest release, the polix of the PATH); without git or outside a repository only the plugin; else the
workflow (the methods `WorkflowProblems`, `NewerHint` and `BaseWarning` of `setup.Inspection`), the requirements
(`requirements`: one `warn` per program of `Workflow.Programs`, the requirements followed by git, that
`Requirements` of the inspection says the PATH does not run, each once and in that order, `<program> is not on the PATH: polix.yaml requires it`,
or one `ok` line, `<n> programs of requirements on the PATH`, when it runs them all; nothing without requirements or
with a workflow that cannot be used; a missing program never fails the report), the rules
(`RuleProblems`, and `runProblems` of the logic: a check that a runner of `rules.run` runs is never reported as not
executable nor without interpreter), the hooks of Claude Code (`logic_hooks.go`: the settings that cannot be read
or are not JSON, every event of `setup.ClaudeHookEvents`, the matchers, the timeouts above the run budget, run once,
the session start update, with `setup.HookHandler.Polix`, `Update` and `Legacy`; an event that does not run polix
fails, but `PostToolUseFailure` and `SubagentStart`, which settings written by an older polix lack, only warn and
ask to run `polix install`, since only the feedback of failed commands or the context at the start of subagents is
lost), the plugin (the version of the manifest written,
`setup.Plugin.Manifest`, against `setup.Plugin.ManifestVersion`, and without `--hook` what `claude plugin list`
said), Codex, and the state.
Codex is checked only when polix is registered for it, as `polix install --agent codex` leaves it: a Codex hooks
file of the project or the user that runs polix, or the plugin enabled in `~/.codex/config.toml`
(`CodexEnabled` of the inspection); otherwise it has no finding. When it is registered, a hooks file that cannot be
read or parsed fails, a plugin not enabled or hooks not registered warn, and else the OK line says the hooks are
registered, that Codex runs them once trusted with `/hooks`, and that Codex support is not implemented and untested.

## Response

`Response`: `Hook`, `Findings` and `Failed`. The command prints one line per finding and exits with 1
on a failure; with `--hook` it prints only the problems, in a paragraph for the agent, and exits with 0.

## Port

`Port`, implemented by `Adapter{OS: runtime.GOOS, Look: exec.LookPath, Rules: rules, Cache: cache, Workflow:
loadWorkflow}`, with the loaders of the rules (`internal/app/rule/load`), of the cache (`internal/app/version/load`)
and of polix.yaml (`internal/app/workflow/load`) that `cmd/polix` builds once and passes to `Command`:

- `Home`, `Executable`, `LookPath` (symlinks resolved) and `SameFile`.
- `LoadCache`, `SaveCache` and `Latest`: the cache of `polix update`, loaded through the loader of the cache and
  saved with the unexported type `cacheJSON` of `out.go`, the shape the loader decodes, and the releases
  (`POLIX_RELEASES_URL`), within 5 seconds.
- `OpenRepo` and `WriteState`.
- `LoadWorkflow`: through the loader of polix.yaml, the workflow, whether it exists and the error of loading it;
  and why none of its base branches exists in the repository, asked to git.
- `LoadRules`: through the loader of the rules, a `rule.Loaded` (every rule, the problems of the invalid ones and of
  the ids repeated, and the error of a file that cannot be read); and, by the check of each rule that has one, whether its file is
  missing and why it cannot run (a check that exists but is not executable; on Windows, without its interpreter).
- `HooksFiles`: the hooks files of an agent that exist, the settings of Claude Code or the hooks files of Codex,
  decoded (`agentconf.ParseHooks`, for which an empty file is not JSON, and `agentconf.ReadHooks`, for which it has
  no hooks) into `setup.HooksFile`: the groups of hooks that run a command, or why the file cannot be read or is
  not JSON.
- `CodexEnabled`: whether `~/.codex/config.toml` enables the plugin (`agentconf.CodexEnabled`).
- `Plugin`: the plugin embedded in the binary, with the version of the manifest of the plugin in the marketplace
  and whether it is the plugin polix (`agentconf.ReadManifest`).
- `ClaudePlugin`: `claude plugin list --json`, read by the decoding type of the adapter: whether polix is installed
  for the user, enabled, and the folder it is read from; `setup.ErrNoCLI` without the CLI.

## Errors

Every problem is a finding; only a broken embedded plugin is an error of the shell.

## Flow

1. `Command` injects the `Adapter`, with the loaders of the rules, of the cache and of polix.yaml it was given.
2. The shell reads the binary, the latest release (`releases`: only a release build without `--hook` nor
   `--offline`, saving it with `Cache.Saw`; the method `parse` of the unexported type `versionText` of
   `in_version.go` translates the version of the request into a `version.Version`, validated with the domain, to
   tell a release build), the PATH, the repository and its facts (`inspectRepository`; for a workflow that can be
   used, `lookRequirements` asks `LookPath` once per program of `Workflow.Programs`, on the PATH the hooks give polix,
   which on Windows tries the extensions of `PATHEXT`, and keeps by program why it is not found), the
   hooks files of both agents and `~/.codex/config.toml`, decoded (from which the logic decides whether polix is
   registered for Codex), the plugin (`readPlugin`) and Claude Code (`askClaude`), into a `setup.Inspection`.
3. `Decide` decides the findings.
4. The command prints the response and sets the exit code.
