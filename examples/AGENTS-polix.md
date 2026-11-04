<!--
Fragment to paste into the AGENTS.md (or CLAUDE.md) of a repository governed by polix. It tells the agent how
to install polix on a machine that lacks it, on any system: the hooks of polix (.claude/settings.json)
only run plain `polix ...` commands, valid in every shell Claude Code uses (sh on macOS and Linux, Git Bash or
PowerShell on Windows), so they cannot detect a missing polix themselves.
-->

## polix

Este repositorio aplica su workflow (`polix.yaml`) y sus reglas (`ai/rules/`) con polix, a través de los hooks
de `.claude/settings.json`. Si polix no está instalado en este equipo, los hooks fallan con un error como `polix: command not found` o `polix : The term 'polix' is not recognized`, y el workflow no se aplica.

Cuando eso ocurra, o el usuario lo pida, propón instalarlo con el comando de su sistema y ejecútalo solo con su
aprobación:

- macOS y Linux (también Windows con Git Bash):
  `curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh`
- Windows (PowerShell):
  `powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"`

Los dos instalan el binario y ejecutan `polix install --user`, que instala para el usuario el plugin `polix`:
en Claude Code, las skills `/polix:workflow`, `/polix:add-rule`, `/polix:doctor`, `/polix:init`,
`/polix:plan`, `/polix:review` y `/polix:audit` y los agents `polix:workflow-author`, `polix:rules-author`,
`polix:reviewer`, `polix:rules-reviewer` y `polix:auditor`.
Los hooks ya vienen en el repositorio.
Después, pide al usuario que reinicie Claude Code para que cargue el plugin y los hooks encuentren polix en el
`PATH`, y comprueba el equipo con `polix doctor`.
