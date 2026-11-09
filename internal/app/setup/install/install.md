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

`Request`: `User`, `Dir`, `Agents` (parsed by the entry with `parseAgents`: trimmed, in lower case and once
each, every name validated by the domain, `setup.Setup.Valid`; empty for Claude Code alone), `Quiet` and `Version`
(`build.Current`).

## Logic

`Decide(asked setup.Setup, embedded map[string][]byte) Output` takes the setup as asked, which the shell builds and
validates (the home, the root, the agents of the request and the running version), and
decides the `setup.Setup` (the agents asked, else Claude Code alone, `Setup.Defaulted`: Codex is never added by
itself), the
`setup.Plugin` of the binary to write to the local marketplace, the `Summary` to confirm, the skills and agents of
the plugin (`Plugin.Skills`, `Plugin.Agents`) and the hooks files; a refusal is its `Failure`.

## Response

`Response`: `Changes`, `Version`, `Agents`, `Skills`, `PluginAgents`, `HooksFiles`, `User`, `Quiet`,
`NotOnPath` and `Duplicates`. The command prints each change (a todo as `run by hand: <command>`); on an error it
stops there with the error. Unless `--quiet` it then tells what was installed and what to do next, with one more
line when the agents include Codex, saying its support is not implemented in this release and the setup is
untested; and it warns on stderr about the other settings of Claude Code that also register the hooks.

## Port

`Port`, implemented by `Adapter`:

- `Home`, `Root` (empty outside a repository) and `OnPath`.
- `Confirm`: with `Ask` (set by the command when a person is at the terminal and `--yes` is not given), asks
  with huh; a refusal is `setup.ErrCancelled`.
- `Plugin`: the files of the plugin embedded in the binary (package `plugins`).
- `MarketplaceFiles`: the files of the local marketplace (`agentconf.Marketplace`): the embedded plugin with its
  `Plugin.ManifestVersion` in its manifests, and a manifest per agent; a manifest of the plugin without version is
  an error.
- `WritePlugin`: syncs the marketplace folder, writing atomically what differs and removing what is stale.
- `RegisterPlugin`: `claude plugin marketplace add` and `claude plugin install` (and `update` when the version
  differs, their answers read by `agentconf.ClaudeAnswer`), or for Codex `codex plugin marketplace add` and the
  plugin enabled in `~/.codex/config.toml` (`agentconf.CodexPlugin`); a CLI that is missing leaves a todo.
- `RegisterHooks`: writes the hooks of polix of an agent (`setup.Setup.Hooks`, given by the shell) into its hooks
  file in place of the ones of polix it had (`setup.HookHandler.Polix`), keeping every other setting and hook
  (`agentconf.MergeHooks`); a file that is not a JSON object is an error that names it.
- `Registered`: which settings of Claude Code register a hook of polix (`agentconf.Registers`), for the
  duplicates.

## Errors

The error of the home, of the plugin, of a setup that is not valid (an unknown agent, no running version) and of
the files of the marketplace stop the install before it asks; the
errors of the agents are joined, so one agent that fails does not keep the hooks of the other from being
registered. A person that does not confirm makes it exit with 1 without a message.

## Flow

1. `Command` parses the agents (`parseAgents`), injects `Adapter{Ask: ask}`.
2. The shell reads the home, the repository unless `--user`, and the plugin, and builds the setup asked.
3. `Decide` decides the setup (Claude Code alone unless agents are asked) and the plugin; the shell asks the port
   for the files of the marketplace, then for confirmation.
4. The shell writes the plugin, registers it and the hooks with each agent, then asks the PATH and the duplicates
   (`setup.Setup.Duplicates` of the settings `Registered`).
5. The command prints the response, with the line on Codex not implemented when Codex is among the agents.
