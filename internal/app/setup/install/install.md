# InstallSetup

Installs what the agents need to run polix, from the binary: the plugin polix for the user, registered with Claude
Code (and with Codex only when asked), the hooks of polix in the repository, and the hooks of git that run
`polix githook` in its local git configuration; with `--githooks`, only those hooks of git. Codex is opt-in: its
support is not implemented in this release and the setup is untested.

## Trigger

```
polix install [--user] [--dir <dir>] [--agent claude,codex] [--githooks] [--yes] [--quiet]
```

- `--user`: only the plugin of the user, not the hooks of the repository.
- `--dir`: the repository whose hooks to register; defaults to the current folder.
- `--agent`: the agents; by default Claude Code alone, and Codex only with `--agent codex` (not implemented).
  Another agent is an error.
- `--githooks`: only the hooks of git of the repository, without plugin, hooks of the agents nor confirmation; the
  `run` of `start` that a polix.yaml with `githooks` expands into. With `--user` or `--agent` it is an error before
  anything runs; `--yes` and `--quiet` are accepted and change nothing.
- `--yes`: do not ask for confirmation; without it a person at a terminal is asked (huh, in the adapter).
- `--quiet`: only print what changed.

## Request

`Request`: `User`, `Dir`, `Agents` (parsed by the entry with `parseAgents`: trimmed, in lower case and once
each, every name validated by the domain, `setup.Setup.Valid`; empty for Claude Code alone), `Quiet`, `Version`
(`build.Current`) and `GitHooks`, set by `--githooks`, which the entry rejects with `--user` or `--agent`.

## Logic

`Decide(asked setup.Setup, embedded map[string][]byte, gitHooksOnly bool) Output` takes the setup as asked, which
the shell builds and validates (the home, the root, the agents of the request and the running version), and
decides the `setup.Setup` (the agents asked, else Claude Code alone, `Setup.Defaulted`: Codex is never added by
itself), the
`setup.Plugin` of the binary to write to the local marketplace, the `Summary` to confirm, the skills and agents of
the plugin (`Plugin.Skills`, `Plugin.Agents`), the hooks files and, inside a repository, `GitHooks`: the hooks of git polix
registers (`setup.GitHooks`, `pre-commit` and `commit-msg`), for any agent and whether or not polix.yaml has
`githooks`; none with `--user` or outside a repository. The `Summary` (`setup.Setup.Summary`) names the plugin
and the hooks file of each agent and, with `GitHooks`, the hooks of git in the local git configuration of the
repository (`hook.polix-pre-commit`, `hook.polix-commit-msg`). A refusal is its `Failure`.

With `gitHooksOnly` (`--githooks`) it decides only `GitHooks` and `GitHooksOnly`, without plugin (the shell reads
none), hooks files nor summary; outside a repository, which `--user` also leaves without a root, its `Failure` is
`not inside a git repository`.

## Response

`Response`: `Changes`, `Version`, `Agents`, `Skills`, `PluginAgents`, `HooksFiles`, `User`, `Quiet`,
`NotOnPath`, `Duplicates`, `Unlisted`, the hooks of git written that git does not list, `Disabled`, the hooks of git
written that git lists as disabled (`hook.<name>.enabled=false`), and `GitHooksOnly`, true with `--githooks`. The
command prints each change (a todo as `run by hand: <command>`, a hook of git as `git config hook.polix-pre-commit`);
on an error it stops there with the error. Unless `--quiet` it then tells what was installed and what to do next,
with one more line when the agents include Codex, saying its support is not implemented in this release and the
setup is untested; and it warns on stderr about the other settings of Claude Code that also register the hooks and,
with `Unlisted`, that git does not run hooks from its configuration (it needs git 2.54 or later): the hooks are kept,
but the githooks of polix.yaml do not apply until git is updated. With `Disabled` it warns that those hooks are
disabled in the git configuration (`hook.polix-pre-commit.enabled false`), so the githooks of their events do not
apply until they are enabled again; install never touches `enabled`.

With `GitHooksOnly` it prints only the changes, nothing when the hooks were already there, and the warnings of
`Unlisted` and `Disabled` on stderr; it exits 1 with the warning of `Unlisted`, so the `run` of `start` that runs it
reaches the agent, and 0 otherwise, also with `Disabled`: a person turned them off and installing again does not
turn them on. Without `--githooks` the install exits 0 also with `Unlisted`.

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
  (`agentconf.MergeHooks`); a file that is not a JSON object is an error that names it. Every handler of polix,
  the update one included, carries the status `setup.StatusMessage`, written as `"statusMessage": "polix"`, so both
  agents show `polix` next to their spinner while the hook runs; a file written without it gets it on the next
  install. Both agents get `SubagentStart`, right after `UserPromptSubmit` in `setup.HookEvents` and without
  matcher, so the start of a subagent reaches the event `subagent_start`; a file of an earlier release gets it on the
  next install, added after its other events.
- `Registered`: which settings of Claude Code register a hook of polix (`agentconf.Registers`), for the
  duplicates.
- `RegisterGitHook`: writes a hook of git into the local configuration of the repository, never that of the user
  (`git.Repo.SetHook`): `hook.<name>.event` replaced by its event, so a second install leaves one, and
  `hook.<name>.command` set to `polix githook <event>`, plain `polix` as the hooks of the agents; `created` when the
  hook had no key, `updated` when it had other values, and no change when it had these. The names are
  `polix-pre-commit` and `polix-commit-msg` (`setup.GitHook.Name`), since git forbids a name equal to an event.
- `Unlisted`: the hooks of git that `git hook list <event>` does not name (`git.Repo.Hooks`); all of them when git
  cannot list hooks, as a git older than 2.54, which ignores the hooks of its configuration without error; and
  apart, those it names as disabled (`disabled<TAB><name>`, from `hook.<name>.enabled=false`).

## Errors

The error of the home, of the plugin, of a setup that is not valid (an unknown agent, no running version) and of
the files of the marketplace stop the install before it asks; the
errors of the agents and of the hooks of git are joined, so one agent that fails does not keep the hooks of the
other, nor those of git, from being registered. A person that does not confirm makes it exit with 1 without a message.
`--githooks` with `--user` or `--agent`, and `--githooks` outside a repository, are errors that exit 1 and write
nothing; with `--githooks` the errors of the two hooks of git are joined, and git not listing them exits 1.

## Flow

1. `Command` rejects `--githooks` with `--user` or `--agent`, parses the agents (`parseAgents`), injects
   `Adapter{Ask: ask}`.
2. The shell reads the home and the plugin unless `--githooks`, and the repository unless `--user`, and builds the
   setup asked.
3. `Decide` decides the setup (Claude Code alone unless agents are asked) and the plugin; with `GitHooksOnly` the
   shell writes the hooks of git of the output (`RegisterGitHook`) and asks which of them git does not list or
   lists as disabled (`Unlisted`), and stops there. Otherwise it asks the port for the files of the marketplace,
   then for confirmation.
4. The shell writes the plugin, registers it and the hooks with each agent, writes the hooks of git of the output
   (`RegisterGitHook`) and asks which of them git does not list or lists as disabled (`Unlisted`), then asks the
   PATH and the duplicates (`setup.Setup.Duplicates` of the settings `Registered`).
5. The command prints the response, with the line on Codex not implemented when Codex is among the agents; with
   `GitHooksOnly`, only the changes and the warnings, and exits 1 when git does not list the hooks.
