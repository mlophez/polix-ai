# UninstallSetup

Removes what polix install wrote: the plugin from the agents and the local marketplace, and the hooks of polix
from the repository, those of git included, keeping every other setting and hook.

## Trigger

```
polix uninstall [--user] [--dir <dir>] [--agent claude,codex] [--yes] [--quiet]
```

The flags are those of `polix install`; without `--agent` both agents, so a Codex set up with `--agent codex`
(Codex support is not implemented) is cleaned up too.

## Request

`Request`: `User`, `Dir`, `Agents`, `Quiet` and `Version`, as for `polix install`. The entry parses `Agents` from
`--agent` with `parseAgents`: trimmed, in lower case and once each, every name validated by the domain
(`setup.Setup.Valid`), so another agent is an error before the shell runs.

## Logic

`Decide(asked setup.Setup) Output` takes the setup as asked, which the shell builds and validates (the home, the
root, the agents of the request and the running version), and decides the `setup.Setup` (the agents asked, else
both, `Setup.Defaulted`), the `Summary` to confirm, `RemovePlugin`, true only when both agents are removed, and
`GitHooks`, the hooks of git of polix (`setup.GitHooks`) when both agents leave a repository: install writes them
for any agent, so they stay while one is still installed, and with `--user` or outside a repository there are none.
The `Summary` (`setup.Setup.Summary`) names the plugin and the hooks file of each agent and, with `GitHooks`, the
hooks of git it removes from the local git configuration of the repository (`hook.polix-pre-commit`,
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

A setup that is not valid (an unknown agent, no running version) stops the uninstall before it asks. The first
error after that stops the uninstall, after printing what changed.

## Flow

1. `Command` parses the agents (`parseAgents`), injects `Adapter{Ask: ask}`.
2. The shell reads the home and the repository unless `--user`, and builds the setup asked.
3. `Decide` decides; the shell asks for confirmation.
4. The shell unregisters each agent and its hooks, removes the hooks of git of the output, then removes the
   marketplace.
5. The command prints the response.
