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

`DiagnoseSetupRequest`: `Dir`, `Hook`, `Offline`, `Version` (`build.Current`) and `Now`, recorded in the cache.

## Logic

`DiagnoseSetup(i Inspection, hook bool) DiagnoseSetupOutput` turns the `Inspection` into findings (`Finding`:
level, area, message), in this order: the binary (version against the latest release, the polix of the PATH);
without git or outside a repository only the plugin; else the workflow (`WorkflowProblems`, `NewerHint`,
`BaseWarning`), the rules (`RuleProblems` and `RunProblems`: a check that a runner of `rules.run` runs is never
reported as not executable nor without interpreter), the hooks of Claude Code (every event, the matchers, the
timeouts above the run budget, run once, the session start update), the plugin (`PluginVersion` against
`ManifestVersion`, and without `--hook` `claude plugin list`), Codex, and the state.
Codex is checked only when polix is registered for it, as `polix install --agent codex` leaves it: a Codex hooks
file of the project or the user that runs polix (`Registered`), or the plugin enabled in `~/.codex/config.toml`
(`CodexEnabled`); otherwise it has no finding. When it is registered, a hooks file that cannot be read or parsed
fails, a plugin not enabled or hooks not registered warn, and else the OK line says the hooks are registered, that
Codex runs them once trusted with `/hooks`, and that Codex support is not implemented and untested.

## Response

`DiagnoseSetupResponse`: `Hook`, `Findings` and `Failed`. The command prints one line per finding and exits with 1
on a failure; with `--hook` it prints only the problems, in a paragraph in Spanish for the agent, and exits with 0.

## Port

`DiagnoseSetupPort`, implemented by `DiagnoseSetupAdapter{OS: runtime.GOOS, Look: exec.LookPath}`:

- `Home`, `Executable`, `LookPath` (symlinks resolved) and `SameFile`.
- `LoadCache`, `SaveCache` and `Latest`: the cache of `polix update` and the releases (`POLIX_RELEASES_URL`),
  within 5 seconds.
- `OpenRepo`, `LoadWorkflow`, `LoadRules`, `Missing`, `Runnable` (a check that exists but is not executable; on Windows, without its interpreter),
  `ResolveBase` and `WriteState`.
- `ReadFiles`: the settings and hooks files, the configuration of Codex and the manifest of the plugin.
- `Plugin`: the plugin embedded in the binary.
- `ClaudePlugins`: `claude plugin list --json`, `ErrNoCLI` without the CLI.

## Errors

Every problem is a finding; only a broken embedded plugin is an error of the shell.

## Flow

1. `DiagnoseSetupCommand` injects `DiagnoseSetupAdapter{}`.
2. The shell reads the binary, the latest release (only a release build without `--hook` nor `--offline`, saving
   it with `Cache.Saw`), the PATH, the repository and its facts, the files (among them the Codex hooks files and
   `~/.codex/config.toml`, from which the logic decides whether polix is registered for Codex), the plugin and
   Claude Code, into an `Inspection`.
3. `DiagnoseSetup` decides the findings.
4. The command prints the response and sets the exit code.
