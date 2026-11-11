# UninstallSetup

Removes what polix install wrote: the plugin from the agents and the local marketplace, and the hooks of polix
from the repository, those of git included, keeping every other setting and hook. Each part goes alone with its
flag, as for `polix install`: `--skills`, `--hooks` and `--githooks`, which combine; none asked is every one.

## Trigger

```
polix uninstall [--skills] [--hooks] [--githooks] [--dir <dir>] [--agent claude,codex] [--yes] [--quiet]
```

The flags are those of `polix install`, without its hidden `--user`; without `--agent` both agents, so a Codex set
up with `--agent codex` (Codex support is not implemented) is cleaned up too. `--githooks` alone with `--agent` is
an error before anything runs.

## Request

`Request`: `Parts` (`setup.Parts`, set by `--skills`, `--hooks` and `--githooks`), `Dir`, `Agents`, `Quiet` and
`Version`, as for `polix install`. The entry parses `Agents` from
`--agent` with `parseAgents`: trimmed, in lower case and once each, every name validated by the domain
(`setup.Setup.Valid`), so another agent is an error before the shell runs.

## Logic

`Decide(asked setup.Setup, parts setup.Parts) Output` takes the setup as asked, which the shell builds and validates
(the home, the root, the agents of the request and the running version), and the parts asked, and decides the
`setup.Setup` (the agents asked, else both, `Setup.Defaulted`), its `Parts` (those asked, else every one,
`Parts.Defaulted`), the `Summary` to confirm, `RemovePlugin`, true only with `Skills` when both agents are removed,
and `GitHooks`, the hooks of git of polix (`setup.GitHooks`) of a repository: with `--githooks` always, and without
parts asked only when both agents leave it, since install writes them for any agent, so they stay while one is still
installed; outside a repository there are none. A part of the repository (`--hooks`, `--githooks`) asked outside one
is its `Failure`, `not inside a git repository`; without parts asked, outside a repository only the plugin goes. The
`Summary` (`setup.Setup.Summary`) names, as the parts ask, the plugin and the hooks file of each agent and the hooks
of git it removes from the local git configuration of the repository (`hook.polix-pre-commit`,
`hook.polix-commit-msg`).

## Response

`Response`: `Changes` and `Quiet`. The command prints each change and, when nothing changed and it
did not fail, `nothing to remove` unless `--quiet`.

## Port

`Port`, implemented by `Adapter`:

- `Home` and `Root`.
- `Confirm`: with `Ask` (set by the command when a person is at the terminal and `--yes` is not given), asks
  with huh; a refusal is `setup.ErrCancelled`.
- `UnregisterPlugin`: `claude plugin uninstall` and `claude plugin marketplace remove`, tolerating what is gone
  (their answers read by `agentconf.ClaudeAnswer`); for Codex `codex plugin marketplace remove` when its
  configuration enables the plugin (`agentconf.CodexEnabled`), and the table of the plugin removed from it
  (`agentconf.CodexPlugin`).
- `UnregisterHooks`: the hooks of polix (`setup.HookHandler.Polix`) removed from a hooks file
  (`agentconf.MergeHooks` with none to write), and the file when they were all it held; a file that is not a JSON
  object is an error that names it.
- `RemovePlugin`: removes the marketplace, and `~/.polix` when it is left empty.
- `UnregisterGitHook`: removes the section of a hook of git (`hook.polix-pre-commit`, `hook.polix-commit-msg`) from
  the local configuration of the repository (`git.Repo.RemoveHook`), reported as `removed` with its key
  (`git config hook.polix-pre-commit`); a hook that is not there is no change, and the other hooks stay.

## Errors

A setup that is not valid (an unknown agent, no running version), and a `Failure` of the logic, stop the uninstall
before it asks. The first
error after that stops the uninstall, after printing what changed.

## Flow

1. `Command` rejects `--githooks` alone with `--agent`, parses the agents (`parseAgents`), injects
   `Adapter{Ask: ask}`.
2. The shell reads the home and, with `--hooks` or `--githooks` or no part asked, the repository, and builds the
   setup asked.
3. `Decide` decides; the shell asks for confirmation.
4. The shell unregisters each agent and its hooks (`removeAgents`, as the parts ask), removes the hooks of git of
   the output, then removes the marketplace.
5. The command prints the response.
