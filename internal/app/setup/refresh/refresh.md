# RefreshSetup

Brings what polix installed in line with a new binary, installing nothing new: the hidden command the new binary
runs after `polix update`.

## Trigger

```
polix __refresh [--quiet] [--hooks]
```

- `--quiet`: one line in Spanish when something changed, for the session start hook.
- `--hooks`: also refresh the hooks of polix of the repository of the working folder.

## Request

`Request`: `Dir` (the working folder), `Quiet`, `Hooks` and `Version` (`build.Current`).

## Logic

`Decide(s setup.Setup, files map[string][]byte, embedded map[string][]byte) Output` decides: `Plugin`
when the manifest of the plugin is in the marketplace (`setup.PluginVersion`), with its `Files`; and `Hooks`, the agents
whose hooks file of the repository registers polix. A refusal is its `Failure`.

## Response

`Response`: `Changes` and `Quiet`. When something changed the command prints, with `--quiet`,
`polix ha actualizado su plugin: se carga en la próxima sesión.`, else each change.

## Port

`Port`, implemented by `Adapter`:

- `Home`, `Root`, `Plugin` and `ReadFiles` (the manifest and the hooks files).
- `WritePlugin`: syncs the marketplace folder.
- `UpdatePlugin`: `claude plugin update` and `codex plugin marketplace upgrade`, when their CLIs are here; their
  failures are ignored.
- `RegisterHooks`: rewrites the hooks of polix of an agent.

## Errors

The errors of the plugin and of the hooks are joined and returned after printing what changed; the command exits
with 1.

## Flow

1. `Command` injects `Adapter{}`.
2. The shell reads the home, with `--hooks` the repository, the plugin and the files.
3. `Decide` decides.
4. The shell writes the plugin, tells the agents when it changed, and rewrites the hooks.
5. The command prints the response.
