# UninstallSetup

Removes what polix install wrote: the plugin from the agents and the local marketplace, and the hooks of polix
from the repository, keeping every other setting and hook.

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

`Decide(home, root string, asked []string, running string) Output` decides the `setup.Setup`, the
`Summary` to confirm and `RemovePlugin`, true only when both agents are removed; a refusal is its `Failure`.

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

## Errors

The first error stops the uninstall, after printing what changed.

## Flow

1. `Command` parses the agents (`parseAgents`), injects `Adapter{Ask: ask}`.
2. The shell reads the home and the repository unless `--user`.
3. `Decide` decides; the shell asks for confirmation.
4. The shell unregisters each agent and its hooks, then removes the marketplace.
5. The command prints the response.
