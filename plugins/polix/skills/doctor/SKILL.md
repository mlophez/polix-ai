---
name: doctor
description: >
  Diagnoses and fixes the setup of polix on this machine and repository: binary, PATH, hooks, plugin, workflow, rules.
  It runs only when the user or the workflow of polix asks for it.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: false
---

# Fixing polix

Find why polix does not apply as expected and fix it with the user.

## Rules

- Apply only the fixes the user approves; propose the commands that install or update before running them.
- Talk to the user in the language of the session.

## References

### Fixes

- **version**: `polix update` installs the latest release; a development build needs `polix update --version <v>`.
- **path**: the folder of the binary is not on the `PATH` the hooks use.
  Add it in the profile of the shell (`~/.local/bin` on macOS and Linux), or reinstall on Windows.
  The Windows installer adds `%LOCALAPPDATA%\Programs\polix` to the user `PATH`; then restart the agent.
- **plugin**: `polix install --user` writes this plugin to `~/.polix/marketplace/` and registers it with each agent.
  A new or changed plugin applies in the next session, or after `/reload-plugins` in Claude Code.
- **hooks**: `polix install` registers the hooks of Claude Code in `.claude/settings.json` of the repository.
  When they are registered twice, remove the `polix ...` hooks of one of the files: each hook would run twice.
  When `SessionStart` does not run `polix update --hook`, or runs the command of an earlier polix, run `polix install`.
- **codex**: Codex support is not implemented and untested.
  The check appears only when polix was set up for Codex with `polix install --agent codex`: a Codex hooks file runs
  polix, or the plugin is enabled in `~/.codex/config.toml`.
  Codex skips new or changed hooks until the user trusts them with `/hooks`.
- **workflow** and **rules**: fix the problems `polix config check` prints.
  The skills `workflow` and `add-rule` of polix know their formats.
  When `polix.yaml` requires a newer polix, run `polix update` and leave `polix.yaml` alone.
  An unknown option may belong to a newer polix: run `polix update` and check again before removing it.
- **runners**: `polix config check` reports a runner of `rules.run` with an empty argv, an empty glob, `{files}` or
  `{dirs}` inside a longer argument, or an unknown placeholder; fix it with the skill `workflow`. A check that no runner matches must be executable, or on Windows
  have its interpreter: add a runner for its kind of file or fix the file.
- **state**: the folder `<git-dir>/polix/` must be writable; never edit it by hand, read it with `polix log`.

### Installing polix

- macOS and Linux, also Windows with Git Bash:
  `curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh`
- Windows, PowerShell:
  `powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"`

Both install the binary and run `polix install --user`.
Then `polix install` in the repository registers its hooks, and the user restarts the agent.

## How to operate

Copy this checklist and track your progress:

```text
Doctor progress:
- [ ] Step 1: Run `polix doctor` from the root of the repository; when polix is missing, propose `Installing polix`.
- [ ] Step 2: Explain each `warn` and `fail` line in a sentence and propose its fix (see `Fixes`).
- [ ] Step 3: Apply the fixes the user approves.
- [ ] Step 4: Run `polix doctor` again until it reports no `fail`.
```

## Output format

- **Fixed**: each problem fixed and how.
- **Pending**: the problems left and what the user must do for each, or "none".
