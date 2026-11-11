# InstallSetup

Installs what the agents need to run polix, from the binary, in three parts that combine, each asked with its
flag, and all of them when none is asked: the plugin polix for the user, registered with Claude Code (and with
Codex only when asked) (`--skills`); the hooks of polix of the agents in the repository (`--hooks`); and the hooks
of git that run `polix githook` in its local git configuration (`--githooks`). Codex is opt-in: its support is not
implemented in this release and the setup is untested.

## Trigger

```
polix install [--skills] [--hooks] [--githooks] [--dir <dir>] [--agent claude,codex] [--yes] [--quiet]
```

- `--skills`: the plugin of the user, with its skills and agents.
- `--hooks`: the hooks of the agents of the repository.
- `--githooks`: the hooks of git of the repository; alone, without confirmation, the `run` of `start` that a
  polix.yaml with `githooks` expands into. Alone with `--agent` it is an error before anything runs; `--yes` and
  `--quiet` are accepted and change nothing.
- Without any of the three, every part; outside a repository, then, only the plugin.
- `--user`, hidden: `--skills`, as polix 0.6 and earlier named it, which `polix update` of those releases runs on
  the new binary.
- `--dir`: the repository whose hooks to register; defaults to the current folder.
- `--agent`: the agents; by default Claude Code alone, and Codex only with `--agent codex` (not implemented).
  Another agent is an error.
- `--yes`: do not ask for confirmation; without it a person at a terminal is asked (huh, in the adapter).
- `--quiet`: only print what changed.

## Request

`Request`: `Parts` (`setup.Parts`: `Skills`, `Hooks` and `GitHooks`, set by their flags; none asked is every one),
`Dir`, `Agents` (parsed by the entry with `parseAgents`: trimmed, in lower case and once each, every name validated
by the domain, `setup.Setup.Valid`; empty for Claude Code alone), `Quiet` and `Version` (`build.Current`).

## Logic

`Decide(asked setup.Setup, embedded map[string][]byte, parts setup.Parts) Output` takes the setup as asked, which
the shell builds and validates (the home, the root, the agents of the request and the running version), and the
parts asked, and decides its `Parts` (those asked, else every one, `Parts.Defaulted`), the `setup.Setup` (the agents
asked, else Claude Code alone, `Setup.Defaulted`: Codex is never added by itself), and, as the parts ask: with
`Skills` the `setup.Plugin` of the binary to write to the local marketplace and its skills and agents
(`Plugin.Skills`, `Plugin.Agents`); with `Hooks`, inside a repository, the hooks files; with `GitHooks`, inside a
repository, `GitHooks`: the hooks of git polix registers (`setup.GitHooks`, `pre-commit` and `commit-msg`), for any
agent and whether or not polix.yaml has `githooks`. The `Summary` to confirm (`setup.Setup.Summary`) names, as the
parts ask, the plugin and the hooks file of each agent and the hooks of git in the local git configuration of the
repository (`hook.polix-pre-commit`, `hook.polix-commit-msg`). A refusal is its `Failure`.

A part of the repository (`--hooks`, `--githooks`) asked outside one is the `Failure` `not inside a git
repository`; without parts asked, outside a repository it decides only the plugin. With only `GitHooks` it decides
only `GitHooks`, without plugin (the shell reads none), hooks files nor summary.

## Response

`Response`: `Changes`, `Version`, `Agents`, `Skills`, `PluginAgents`, `HooksFiles`, `Parts`, the parts written,
`Quiet`, `NotOnPath`, `Duplicates`, `Unlisted`, the hooks of git written that git does not list, and `Disabled`, the
hooks of git written that git lists as disabled (`hook.<name>.enabled=false`). The command prints each change (a
todo as `run by hand: <command>`, a hook of git as `git config hook.polix-pre-commit`); on an error it stops there
with the error. Unless `--quiet` it then tells what was installed and what to do next: the skills and agents with
the plugin, the hooks files with the hooks, and with the hooks outside a repository that it is not inside one; with
one more line when the agents include Codex, saying its support is not implemented in this release and the setup is
untested. It warns on stderr about the other settings of Claude Code that also register the hooks and, with
`Unlisted`, that git does not run hooks from its configuration (it needs git 2.54 or later): the hooks are kept, but
the githooks of polix.yaml do not apply until git is updated. With `Disabled` it warns that those hooks are
disabled in the git configuration (`hook.polix-pre-commit.enabled false`), so the githooks of their events do not
apply until they are enabled again; install never touches `enabled`.

With only the hooks of git (`Parts.OnlyGitHooks`) it prints only the changes, nothing when the hooks were already
there, and the warnings of `Unlisted` and `Disabled` on stderr; it exits 1 with the warning of `Unlisted`, so the
`run` of `start` that runs it reaches the agent, and 0 otherwise, also with `Disabled`: a person turned them off and
installing again does not turn them on. With other parts the install exits 0 also with `Unlisted`.

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

The error of the home, of the plugin, of a setup that is not valid (an unknown agent, no running version), the
`Failure` of the logic and the error of the files of the marketplace stop the install before it asks; the errors of
the agents and of the hooks of git are joined, so one agent that fails does not keep the hooks of the other, nor
those of git, from being registered. A person that does not confirm makes it exit with 1 without a message.
`--githooks` alone with `--agent`, and `--hooks` or `--githooks` outside a repository, are errors that exit 1 and
write nothing; with only `--githooks` the errors of the two hooks of git are joined, and git not listing them exits
1.

## Flow

1. `Command` rejects `--githooks` alone with `--agent`, parses the agents (`parseAgents`), injects
   `Adapter{Ask: ask}`.
2. The shell reads the home with the plugin or the hooks of the agents, the plugin of the binary with the plugin,
   and the repository with the hooks of the agents or of git, and builds the setup asked.
3. `Decide` decides the parts, the setup (Claude Code alone unless agents are asked) and the plugin; with only the
   hooks of git the shell writes them (`RegisterGitHook`) and asks which of them git does not list or lists as
   disabled (`Unlisted`), and stops there (`installGitHooks`). Otherwise (`installParts`) it asks the port for the
   files of the marketplace with the plugin, then for confirmation.
4. The shell writes the plugin and registers it with each agent with `Skills`, registers the hooks of each agent
   with `Hooks`, writes the hooks of git of the output (`RegisterGitHook`) and asks which of them git does not list
   or lists as disabled (`Unlisted`), then asks the PATH and, with the hooks of the agents, the duplicates
   (`setup.Setup.Duplicates` of the settings `Registered`).
5. The command prints the response (`in_print.go`), with the line on Codex not implemented when Codex is among the
   agents; with only the hooks of git, only the changes and the warnings, and exits 1 when git does not list them.
