# Changelog

All notable changes to polix are recorded in this file.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and polix follows
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).
A 0.x release may still change the format of `polix.yaml`; `requires:` lets a repository pin the oldest polix it needs.

## [Unreleased]

### Fixed

- A commit in another repository, as `git -C ../other commit` run from a session in this one, no longer fires the
  entries of `pre_commit` of this repository; `protected_files` and `forbidden_commands` still apply to it.
  Only a line of plain `git [-C <folder>] add|commit` commands, with nothing the shell expands, whose commit folders
  are outside this repository's folder and inside another existing repository counts as such (on Windows, `-C`
  folders written with their drive); anything else, a linked worktree of this repository included, keeps the gates.

## [0.1.0] - 2026-11-08

The first public release: declarative guardrails for Claude Code, where one `polix.yaml` decides what the agent may
not touch, what must pass and what runs at every step.
Claude Code is the supported agent; Codex support is not implemented.

### Added

- The engine, `polix hook claude`, the entry point of the Claude Code hooks: it reads each hook call on stdin, blocks
  with the reasons for the agent, injects context and runs commands, in milliseconds per call.
- A built-in protection of the state of polix (`<git-dir>/polix/`), applied whatever the workflow says.
- `polix.yaml` `version: 1`, with `requires:` to pin the oldest polix a repository needs.
- `guardrails:`, the basics as five short lists: `protected_files`, `forbidden_commands`, `protected_branches`,
  `format_on_edit` and `run_before_done`.
- `on:`, the entries of each event (`start`, `prompt`, `pre_edit`, `post_edit`, `pre_command`, `post_command`,
  `pre_commit`, `subagent_stop` and `stop`), each with `when`, `unless` and one action.
- The conditions `branch`, `var`, `path`, `changed`, `file`, `contains`, `command`, `writes`, `exit`, `actor`,
  `finished`, `changed_lines`, `changed_files`, `risk_score`, `not` and `any`.
- The verbs `block` (with `once` for a reminder), `run` and `inject` (with `always`), the placeholders, `vars` taken
  from the branch name, the built-in `guidance` and a loop guard on stop (`max_stop_retries`).
- Rules in `ai/rules/*.yaml` (or `rules.path`): `inject: { rules }` injects only the rules whose scope matches the
  edited path, `run: { rules: changed }` runs the checks of the deterministic rules a change touches, and
  `polix rules check` (with `--changed` and `--base`) runs them in CI.
- `polix config check`, which validates `polix.yaml` and the rules, and `polix config fmt`, which formats
  `polix.yaml` in place keeping every comment (`--check` only reports).
- `polix graph`, which prints the workflow as a Mermaid diagram.
- `polix log`, which prints the last entries of the state log (`--tail`, `--json`).
- `polix risk`, which prints the lines, files and risk score of a change file by file (`--changed`, `--commit`),
  weighted by `risk.weights`.
- `polix doctor`, which checks that the machine and the repository are ready for polix (`--hook`, `--offline`).
- `polix install` and `polix uninstall`, which set up and remove the plugin of the user and the hooks of the
  repository in `.claude/settings.json`.
- Self update: `polix update` replaces the binary with a checked release and refreshes the plugin (`--check`,
  `--version`), and the session start hook runs it at most once an hour.
- Static binaries for Linux, macOS and Windows on amd64 and arm64, published with `checksums.txt`, and the
  installers `install.sh` and `install.ps1`, which check the download before installing it.
- The plugin `polix` for Claude Code, embedded in the binary: the skills `workflow`, `add-rule`, `doctor`, `init`,
  `plan`, `review` and `audit`, and the agents `workflow-author`, `rules-author`, `builder`, `reviewer`,
  `rules-reviewer` and `auditor`, each run only when the user or the workflow asks for it.

### Known limits

- Codex is not implemented: `polix hook codex` and `polix install --agent codex` exist, but were never tested
  against a running Codex, and `polix install` sets up only Claude Code unless `--agent codex` is passed.
- Releases are not signed: `checksums.txt` protects against a corrupt or swapped download, not against someone who
  can publish releases of the repository.
- The condition `pr`, on the state of the pull request, is not implemented.
- The rest of the known limits are in [docs/polix.md](docs/polix.md#known-limits).

[Unreleased]: https://github.com/mlophez/polix-ai/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/mlophez/polix-ai/releases/tag/v0.1.0
