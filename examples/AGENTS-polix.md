<!--
Fragment to paste into the AGENTS.md (or CLAUDE.md) of a repository governed by polix, for Claude Code. It tells
the agent how to install polix on a machine that lacks it, on any system: the hooks of polix (.claude/settings.json)
only run plain `polix ...` commands, valid in every shell Claude Code uses (sh on macOS and Linux, Git Bash or
PowerShell on Windows), so they cannot detect a missing polix themselves.
-->

## polix

This repository enforces its workflow (`polix.yaml`) and its rules (`ai/rules/`) with polix, through the Claude Code
hooks of `.claude/settings.json`.
When polix is not installed on this machine, the hooks fail with an error such as `polix: command not found` or
`polix : The term 'polix' is not recognized`, and the workflow is not enforced.

When that happens, or the user asks for it, offer to install it with the command of their system and run it only
with their approval:

- macOS and Linux (also Windows with Git Bash):
  `curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh`
- Windows (PowerShell):
  `powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"`

Both install the binary and run `polix install --skills`, which installs the plugin `polix` for the user:
in Claude Code, the skills `/polix:workflow`, `/polix:add-rule`, `/polix:risk`, `/polix:doctor`, `/polix:init`,
`/polix:task`, `/polix:plan`, `/polix:build`, `/polix:review`, `/polix:audit`, `/polix:writing-skills` and
`/polix:writing-agents` and the agents `polix:workflow-author`,
`polix:rules-author`, `polix:builder`, `polix:reviewer`, `polix:rules-reviewer` and `polix:auditor`.
Run those skills and agents only when the user or the workflow of polix asks for them, never on your own initiative.
The hooks already come with the repository.
Then ask the user to restart Claude Code so it loads the plugin and the hooks find polix on the `PATH`, and check
the machine with `polix doctor`.
