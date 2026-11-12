# Contributing to Polix

Thanks for your interest in Polix. Bug reports, ideas and pull requests are all welcome.
This guide explains how to report an issue, how to send a change, how to build, test and check the repository, and
how its own Polix governs your session.
Code, comments, docs and commit messages are written in English.

## Ways to contribute

- **Report a bug.** Open an [issue](https://github.com/mlophez/polix-ai/issues) with your system, the output of
  `polix --version` and `polix doctor`, the smallest `polix.yaml` that reproduces it, and what you expected.
  `polix log` shows what the hooks decided, which usually tells where it went wrong.
- **Propose a feature.** Open an issue first and describe the problem it solves before writing code: a new key of
  `polix.yaml` is a format change, and it is easier to agree on the shape before the implementation.
- **Share a recipe.** A `polix.yaml` that solves a real problem for your team is a good addition to
  [docs/use-cases.md](docs/use-cases.md).
- **Improve the docs.** Typos, unclear explanations and missing examples are fixed with a pull request, no issue
  needed.
- **Test Codex.** Codex support exists but has never run against a real Codex; reports from Codex users help a lot.

Security issues are not reported in public issues: see [SECURITY.md](SECURITY.md).
Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).

## Sending a pull request

1. Fork the repository and create a branch from `main`.
2. Make the change with its tests, and update the docs it affects (see [Docs](#architecture-and-docs)).
3. Run `just test`, `golangci-lint run ./...` and `just check` locally.
4. Add a line under `Unreleased` in [CHANGELOG.md](CHANGELOG.md) if the change is notable for users.
5. Open the pull request against `main` and explain what it changes and why.

Keep each pull request to one change. CI must pass on Linux, macOS and Windows before it is merged.

Commit messages are one imperative sentence that says what changes and why, without a prefix: for example,
`Run install --user from polix update and the installers, so the plugin is installed for the user`.

**You do not need Claude Code to contribute.** The engine is plain Go, and the tests, lint and checks run without
it. Claude Code is only needed to work under the repository's own Polix, to validate the plugin or to record the
demo.

## Requirements

- Go, at the version of `go.mod`.
- [just](https://github.com/casey/just), whose recipes run with `sh`: on Windows, run them from Git Bash.
- [golangci-lint](https://golangci-lint.run/), at the version pinned in `.github/workflows/ci.yml`.
- git.
- Optional: Claude Code, to work on the repository with its own Polix and to validate the plugin.
- Optional: [vhs](https://github.com/charmbracelet/vhs), with ttyd and ffmpeg, only to regenerate the demo.

## Build, test and check

```bash
just build                            # build polix into build/polix
just install                          # go install, the binary the hooks run
just test                             # go vet and go test of the whole module
just test -run TestName               # one test
golangci-lint run ./...               # lint with .golangci.yml, as CI does
golangci-lint fmt                     # format with gofmt and goimports
just check                            # install, then polix config check, config fmt --check,
                                      # rules fmt --check, rules check and doctor --offline
claude plugin validate plugins/polix  # validate the plugin
```

CI (`.github/workflows/ci.yml`) runs vet, lint, the tests and `polix rules check` on Linux, macOS and Windows, on
every push to `main` and every pull request.
The tests of `install.sh` and `install.ps1` are Go tests in `tests/install/`, and on Windows they run under
Windows PowerShell 5.1 and PowerShell 7.

## The repository is governed by its own Polix

This repository uses Polix on itself:

- `polix.yaml` is its workflow.
- `ai/rules/` holds the rules of its code (`architecture.yaml`, `go.yaml`, `platform.yaml`), and `retired.yaml`
  keeps the ids of the retired ones, which Polix never reads.
- `.claude/settings.json` registers the Polix hooks for Claude Code, committed with the repository.

The hooks run `polix` from the `PATH`, so run `just install` once before opening a Claude Code session here, and
again after every change to the engine, so the hooks run your build.
Until then, the hooks fail with "command not found", which Claude Code treats as a non-blocking error, and the
workflow does not apply.
`polix install --skills` adds the `polix` plugin (its skills and agents) for your user.

In a session, the workflow of `polix.yaml`:

- protects `AGENTS.md` and `CLAUDE.md`: only a person changes the instructions of the repository;
- formats each edited Go file with `golangci-lint fmt`, and `polix.yaml` with `polix config fmt`;
- runs `golangci-lint run ./...` and `go test ./...` before the agent ends its turn, when the files they cover
  changed;
- blocks the commit of a large or risky Go change until the `polix:reviewer` agent has reviewed it;
- reminds once, before a commit, to check whether `README.md` or `docs/` need an update.

`polix log` prints the decisions of the hooks, and `polix doctor` checks the setup of your machine.

## Architecture and docs

- [docs/architecture.md](docs/architecture.md): the layout of the Go code, `internal/app/<noun>/<verb>/` and
  `internal/platform/<lib>/`, where each command is a use case package with an entry, a pure logic and an adapter.
  Its rules are GOARCH-1 to GOARCH-45 of `ai/rules/architecture.yaml`, and the deterministic ones are checked by
  the tests of `tests/architecture/`, which run golangci-lint for the import lists of depguard (GOARCH-41 to
  GOARCH-43) and for the limits of size and complexity of GO-4 of `ai/rules/go.yaml` (gocyclo, gocognit and the
  rule `file-length-limit` of revive).
- [docs/polix.md](docs/polix.md): how Polix behaves, its commands and its known limits.
- [docs/reference.md](docs/reference.md): every key, event, condition and verb of `polix.yaml`.
- [docs/use-cases.md](docs/use-cases.md): recipes of `polix.yaml`.

A change to the behaviour, the commands or the format of `polix.yaml` updates `docs/polix.md` in the same change,
and a change to the format also updates `examples/polix.yaml`.
`plugins/polix/skills/workflow/reference.yaml` is a copy of `examples/polix.yaml`, and a test keeps both equal.
Add a line for each notable change under `Unreleased` in [CHANGELOG.md](CHANGELOG.md).

## The demo

`docs/assets/demo.gif`, the demo of the README, is a real Claude Code session recorded with vhs from
`docs/assets/demo/demo.tape`, in a scratch Go module governed by `docs/assets/demo/polix.yaml`.
Its reviewer is `docs/assets/demo/reviewer.md`, a read-only agent of the demo that never stops the recording on a
permission prompt.
Regenerate it with `just demo` from the root of the repository, which needs vhs, ttyd, ffmpeg, golangci-lint and
Claude Code logged in.
The recipe records `docs/assets/demo.mp4` in real time (ignored by git) and turns it into the GIF, sped up and
without its idle frames, at 1.5 times its speed by default; `just demo 2` speeds it up more.
`just demo-gif <speed>` turns the last recording into the GIF again, without spending a new session.
The session is real, so every recording differs and spends tokens: watch the GIF before committing it.
Regenerate it only when the demo needs to change, and keep it under 2 MB, since every version stays in the history
of the repository.

## Releases (maintainers)

Only maintainers cut releases. A release is cut from a clean `main` that matches `origin/main`:

```bash
just release 0.2.0
```

It runs the tests, tags `v0.2.0` and pushes the tag.
The release workflow (`.github/workflows/release.yml`) then runs CI on the three systems, builds the assets with
`scripts/dist.sh`, publishes the GitHub release and installs it on Linux, macOS and Windows with the documented
installers.
`just dist <version>` builds the same assets into `dist/` locally.
Before tagging, give the `Unreleased` part of [CHANGELOG.md](CHANGELOG.md) the version and the date of the release.
The tag of a published release is never moved: a broken release is fixed on `main` and released again as a new
version.
