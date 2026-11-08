# Contributing to polix

Thanks for your interest in polix.
This guide explains how to build, test and check the repository, how its own polix governs your session, and how a
release is cut.
Code, comments, docs and commit messages are written in English.

## Requirements

- Go, at the version of `go.mod`.
- [just](https://github.com/casey/just), whose recipes run with `sh`: on Windows, run them from Git Bash.
- [golangci-lint](https://golangci-lint.run/), at the version pinned in `.github/workflows/ci.yml`.
- git.
- Optional: Claude Code, to work on the repository with its own polix and to validate the plugin.
- Optional: [vhs](https://github.com/charmbracelet/vhs), with ttyd and ffmpeg, only to regenerate the demo.

## Build, test and check

```bash
just build                            # build polix into build/polix
just install                          # go install, the binary the hooks run
just test                             # go vet and go test of the whole module
just test -run TestName               # one test
golangci-lint run ./...               # lint with .golangci.yml, as CI does
golangci-lint fmt                     # format with gofmt and goimports
just check                            # install, then polix config check, config fmt --check, rules check
                                      # and doctor --offline
claude plugin validate plugins/polix  # validate the plugin
```

CI (`.github/workflows/ci.yml`) runs vet, lint, the tests and `polix rules check` on Linux, macOS and Windows, on
every push to `main` and every pull request.
The tests of `install.sh` and `install.ps1` are Go tests in `tests/install/`, and on Windows they run under
Windows PowerShell 5.1 and PowerShell 7.

## The repository is governed by its own polix

This repository uses polix on itself:

- `polix.yaml` is its workflow.
- `ai/rules/` holds the rules of its code (`architecture.yaml`, `go.yaml`, `platform.yaml`).
- `.claude/settings.json` registers the hooks of polix for Claude Code, committed with the repository.

The hooks run `polix` from the `PATH`, so run `just install` once before opening a Claude Code session here, and
again after every change of the engine, so the hooks run your build.
Until then, the hooks fail with "command not found", which Claude Code treats as a non blocking error, and the
workflow does not apply.
`polix install --user` adds the plugin of polix (its skills and agents) for your user.

In a session, the workflow of `polix.yaml`:

- protects `AGENTS.md` and `CLAUDE.md`: only a person changes the instructions of the repository;
- formats each edited Go file with `golangci-lint fmt`, and `polix.yaml` with `polix config fmt`;
- runs `golangci-lint run ./...` and `go test ./...` before the agent ends its turn, when the files they cover
  changed;
- blocks the commit of a large or risky Go change until the `polix:reviewer` agent has reviewed it;
- reminds once, before a commit, to check whether `README.md` or `docs/` need an update.

`polix log` prints the decisions of the hooks, and `polix doctor` checks the setup of your machine.

## Architecture and docs

- [docs/architecture.md](docs/architecture.md): the layout of the Go code, where each command is a use case with an
  entry, a pure logic and an adapter.
  Its rules are GOARCH-1 to GOARCH-40 of `ai/rules/architecture.yaml`, and the deterministic ones are checked by
  the tests of `tests/architecture/`.
- [docs/polix.md](docs/polix.md): how polix behaves, its commands and its known limits.
- [docs/reference.md](docs/reference.md): every key, event, condition and verb of `polix.yaml`.
- [docs/use-cases.md](docs/use-cases.md): recipes of `polix.yaml`.

A change of the behaviour, the commands or the format of `polix.yaml` updates `docs/polix.md` in the same change,
and a change of the format also updates `examples/polix.yaml`.
`plugins/polix/skills/workflow/reference.yaml` is a copy of `examples/polix.yaml`, and a test keeps both equal.
Add a line for each notable change under `Unreleased` in [CHANGELOG.md](CHANGELOG.md).

## The demo

`docs/assets/demo.gif`, the demo of the README, is recorded with vhs from `docs/assets/demo/demo.tape`: real
`polix hook claude` calls fed with the recorded hook inputs of `docs/assets/demo/`.
Regenerate it with `just demo` from the root of the repository, which needs vhs, ttyd and ffmpeg.
Regenerate it only when the output of the demo changes, and keep it under 2 MB, since every version stays in the
history of the repository.

## Releases

A release is cut from a clean `main` that matches `origin/main`:

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
