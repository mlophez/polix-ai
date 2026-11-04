# polix-ai

`polix` is the engine that connects Claude Code and Codex to the workflow (`polix.yaml`) and the rules
(`ai/rules/`) of a repository through their hooks. It is a Go module (`github.com/mlophez/polix-ai`) shipped as
one static binary per system. Design, commands and limits: `docs/polix.md`.

- Entry point: `cmd/polix`. Packages in `internal/`: `agent` (Claude Code and Codex adapters), `engine`,
  `workflow` (polix.yaml), `rules` (ai/rules), `state` (log in `<git-dir>/polix/`), `cli`, `setup` (polix install:
  the plugin and the hooks), `update`, `version` (comparison of versions), `gitx`, `glob`, `shell`.
- `plugins/polix/` is the plugin of polix for Claude Code and Codex (skills, agents), embedded in the binary by the
  package `plugins`, and it only arrives with `polix install`. Check it with `claude plugin validate plugins/polix`.
- `examples/` holds the reference `polix.yaml`, hook settings and the AGENTS.md fragment for governed repos.
- It must install and run on Linux, macOS and Windows; CI tests the three (`.github/workflows/ci.yml`).
- Recipes: `just test` (vet + tests), `just build`, `just install` (`go install`, the binary the hooks run),
  `just release <version>`.

This repository is governed by its own polix: `polix.yaml` and `ai/rules/` at the root, hooks in
`.claude/settings.json`. After changing the engine, run `just install` so the hooks use the new binary.

## polix

Este repositorio aplica su workflow (`polix.yaml`) y sus reglas (`ai/rules/`) con polix, a través de los hooks de
`.claude/settings.json`. Si polix no está instalado en este equipo, los hooks fallan con un error como
`polix: command not found` o `polix : The term 'polix' is not recognized`, y el workflow no se aplica.

Cuando eso ocurra, o el usuario lo pida, propón instalarlo con el comando de su sistema y ejecútalo solo con su
aprobación:

- macOS y Linux (también Windows con Git Bash):
  `curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh`
- Windows (PowerShell):
  `powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"`
- Desde este mismo repositorio: `just install`.

Después, pide al usuario que reinicie Claude Code para que los hooks encuentren polix en el `PATH`, y comprueba
el equipo con `polix doctor`.
