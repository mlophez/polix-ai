# Review of the preparation of polix 0.1.0

Date: 2026-11-08.
Base: `b4cdafa` on `main`, with the whole change uncommitted in the working tree.
Intent, given: prepare polix 0.1.0 for public release following `ai/plans/release-0-1-0.md`, tasks 1 to 8.
Codex becomes opt-in and is documented everywhere as not implemented.
The docs, README and examples drop "proof of concept" and the internal Logalty references.
The repository gains a demo GIF made with vhs, `CHANGELOG.md` and `CONTRIBUTING.md`.
Out of the plan: the rename of `TestAnswerHookLogaltyFlow`, the install help example and the Codex plugin manifest.

## Verdict

Approved with comments.

## Findings

- minor, consistency, `internal/setup/uninstall_setup_in.go:61` and `:83`: the help of `polix uninstall` was not updated.
  Its long help still presents Codex as a regular agent ("Unregister the plugin polix from Claude Code and Codex").
  Its flag `--agent` keeps the text this change fixed in install: "default: claude, and codex when it is installed".
  That text was already wrong: `uninstall_setup_logic.go` always uses Claude and Codex when no agent is passed.
  It now also contradicts `docs/polix.md`, which says uninstall without `--agent` removes the parts of both agents.
  Evidence: `polix uninstall --help`, built from the current tree, prints that `--agent` default.
  Fix: the flag says the default is both agents and that codex is not implemented, and the long help marks Codex as
  not implemented, as in install.

## Checks

- `go vet ./...`: no output.
- `go test ./...`: every package passes, `internal/setup`, `internal/hook`, `plugins`, `tests/architecture` and
  `tests/install` included.
- `golangci-lint run ./...`: 0 issues.
- `polix rules check --changed`: 32 checks, 0 failed.
- With a binary built in the scratchpad: `polix config check` ok with 45 rules, `polix config fmt --check` ok,
  `polix rules check` 32 checks and 0 failed, `polix doctor --offline` with no Codex line.
- `claude plugin validate plugins/polix`: validation passed.
- `cmp examples/polix.yaml plugins/polix/skills/workflow/reference.yaml`: equal (AC9).
- `rg -n -i "proof of concept|Logalty|Dispatcher de"` over the public files: no match (AC4).
- Local links and anchors of README.md, CHANGELOG.md, CONTRIBUTING.md, docs/polix.md and docs/use-cases.md: all resolve.
- `polix --help`, `polix hook --help` and `polix install --help`: the new texts (AC3); the README lists every command
  of `polix --help`, `risk` included (AC6).
- The demo, replayed by hand in a scratch repository: config check ok, the force push and the write of `.env` exit 2
  with their reasons, `go test ./...` exits 0; the GIF is 1000x600, 24.4 s and 156 KB (AC7).
- The quick start of the README validates, and its `polix hook claude` example exits 2 with the reason.
- `polix install` in a scratch home and repository: without `--agent` only `.claude/settings.json`; with
  `--agent codex` `.codex/hooks.json`, `~/.codex/config.toml` and the not implemented line; then doctor gives the
  Codex OK line with "not implemented and untested" (AC1, AC2).
- `xmllint --noout docs/assets/overview.svg`: valid.
- The facts of CHANGELOG.md and CONTRIBUTING.md checked against the plugin, the rules, the Justfile, `ci.yml`,
  `release.yml`, `tests/install/`, `polix.yaml` and the known limits of `docs/polix.md`.

## Held

- DOC-1: the behaviour changes of `internal/setup` and `internal/hook` update `docs/polix.md` in the same change.
- GOARCH-21 and GOARCH-36: `install_setup.md`, `diagnose_setup.md` and `answer_hook.md` follow the code.
- GOARCH-20 and GOARCH-22: the shell `installSetup` calls the logic once, and the Codex line is printed from the
  response.
- GOARCH-23 and GOARCH-31: `InstallSetup` stays a pure function with prefixed names.
- GOARCH-18: `diagnoseSetupCodex` decides only from `Inspection.Files`, with no I/O.
- GOARCH-30: `Inspection.Valid` stays consistent without `CodexUsed`.
- GO-2: code, comments, docs, examples and the demo are in English.
- GO-3: no new nolint and no change to `.golangci.yml`.
- XPLAT-1: no new path or OS specific behaviour.
- XPLAT-2: the examples add no hook command.
- The deterministic GOARCH rules pass with `polix rules check`.

## Pending

- Task 9 (tag, release, GitHub description and topics, installer check) and AC10: not part of the change.
- `just check` was not run as is, since it replaces the binary the hooks of the machine run; its four commands ran
  with a scratch binary.
- `just demo` was not run, since it rewrites `docs/assets/demo.gif`; its commands were replayed by hand.
- The rendering of the README on GitHub (GIF, SVG in light and dark, anchors as GitHub generates them) was not seen.
- CI on Linux and Windows (PowerShell 5.1 and 7) was not run; only macOS.
- `AGENTS.md`, protected, is stale and must be changed by a person before task 9: it says "Claude Code and Codex",
  places the installer tests in `internal/cli/` instead of `tests/install/`, and its `just check` omits
  `config fmt --check`.
- `.claude/worktrees/` is untracked and not ignored: a `git add --all` would include it in the commit.
