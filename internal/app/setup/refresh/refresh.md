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

`Decide(s setup.Setup, p setup.Plugin, registered map[string]bool) Output` decides: `Plugin` when the
manifest of the plugin polix is in the marketplace (`setup.Plugin.Written`), with the plugin of the binary to write,
`Marketplace`; and `Hooks`, the agents whose hooks file of the repository registers polix (`registered`, by path).

## Response

`Response`: `Changes` and `Quiet`. When something changed the command prints, with `--quiet`,
`polix ha actualizado su plugin: se carga en la próxima sesión.`, else each change.

## Port

`Port`, implemented by `Adapter`:

- `Home`, `Root` and `Plugin`.
- `ReadManifest`: the version of the manifest of the plugin in the marketplace and whether it is the plugin polix
  (`agentconf.ReadManifest`).
- `Registered`: which hooks files of the repository register a hook of polix (`agentconf.Registers`,
  `setup.HookHandler.Polix`).
- `MarketplaceFiles`: the files of the local marketplace (`agentconf.Marketplace`), as for `polix install`.
- `WritePlugin`: syncs the marketplace folder.
- `UpdatePlugin`: `claude plugin update` and `codex plugin marketplace upgrade`, when their CLIs are here and, for
  Codex, its configuration enables the plugin (`agentconf.CodexEnabled`); their failures are ignored.
- `RegisterHooks`: rewrites the hooks of polix of an agent (`setup.Setup.Hooks`, given by the shell) in its hooks
  file (`agentconf.MergeHooks`).

## Errors

The error of the files of the marketplace stops the refresh before anything is written. The errors of writing the
plugin and the hooks are joined and returned after printing what changed; the command exits with 1.

## Flow

1. `Command` injects `Adapter{}`.
2. The shell reads the home, with `--hooks` the repository, the plugin, its manifest in the marketplace and the
   hooks files that register polix.
3. `Decide` decides.
4. The shell asks the files of the marketplace, writes the plugin, tells the agents when it changed, and rewrites
   the hooks.
5. The command prints the response.
