# InstallSetup

Installs what the agents need to run polix, from the binary: the plugin polix for the user, registered with Claude
Code (and with Codex only when asked), and the hooks of polix in the repository. Codex is opt-in: its support is not
implemented in this release and the setup is untested.

## Trigger

```
polix install [--user] [--dir <dir>] [--agent claude,codex] [--yes] [--quiet]
```

- `--user`: only the plugin of the user, not the hooks of the repository.
- `--dir`: the repository whose hooks to register; defaults to the current folder.
- `--agent`: the agents; by default Claude Code alone, and Codex only with `--agent codex` (not implemented).
  Another agent is an error.
- `--yes`: do not ask for confirmation; without it a person at a terminal is asked (huh, in the adapter).
- `--quiet`: only print what changed.

## Request

`InstallSetupRequest`: `User`, `Dir`, `Agents` (parsed by `ParseAgents`; empty for Claude Code alone), `Quiet` and
`Version` (`build.Current`).

## Logic

`InstallSetup(home, root string, asked []string, running string, embedded map[string][]byte) InstallSetupOutput`
decides the `Setup` (the agents asked, else Claude Code alone: Codex is never added by itself), the files of the
local marketplace (`MarketplaceFiles`: the embedded plugin with its `ManifestVersion` and a manifest per agent),
the `Summary` to confirm, the skills and agents of the plugin and the hooks files; a refusal is its `Failure`.

## Response

`InstallSetupResponse`: `Changes`, `Version`, `Agents`, `Skills`, `PluginAgents`, `HooksFiles`, `User`, `Quiet`,
`NotOnPath` and `Duplicates`. The command prints each change (a todo as `run by hand: <command>`); on an error it
stops there with the error. Unless `--quiet` it then tells what was installed and what to do next, with one more
line when the agents include Codex, saying its support is not implemented in this release and the setup is
untested; and it warns on stderr about the other settings of Claude Code that also register the hooks.

## Port

`InstallSetupPort`, implemented by `InstallSetupAdapter`:

- `Home`, `Root` (empty outside a repository) and `OnPath`.
- `Confirm`: with `Ask` (set by the command when a person is at the terminal and `--yes` is not given), asks
  with huh; a refusal is `ErrCancelled`.
- `Plugin`: the files of the plugin embedded in the binary (package `plugins`).
- `WritePlugin`: syncs the marketplace folder, writing atomically what differs and removing what is stale.
- `RegisterPlugin`: `claude plugin marketplace add` and `claude plugin install` (and `update` when the version
  differs), or for Codex `codex plugin marketplace add` and the plugin enabled in `~/.codex/config.toml`; a CLI
  that is missing leaves a todo.
- `RegisterHooks`: merges the hooks of polix into the hooks file of an agent (`RegisterHooks` of the domain).
- `ReadFiles`: the settings of Claude Code, for the duplicates.

## Errors

The error of the home, of the plugin and of the marketplace stop the install; the errors of the agents are
joined, so one agent that fails does not keep the hooks of the other from being registered. A person that does
not confirm makes it exit with 1 without a message.

## Flow

1. `InstallSetupCommand` parses the agents, injects `InstallSetupAdapter{Ask: ask}`.
2. The shell reads the home, the repository unless `--user`, and the plugin.
3. `InstallSetup` decides the setup (Claude Code alone unless agents are asked) and the files; the shell asks for
   confirmation.
4. The shell writes the plugin, registers it and the hooks with each agent, then asks the PATH and the duplicates.
5. The command prints the response, with the line on Codex not implemented when Codex is among the agents.
