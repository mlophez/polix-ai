# Polix AI

<!-- Fixed structure, general context only.
Never add, remove, rename or reorder the # and ## headings of this file, nor add ### subsections.
Every section holds only bullets, one fact per item, with exact values.
Every item is a short summary on a single line of at most 120 characters, never wrapped; the detail belongs in docs/.
It describes how the project is; the rules on how to write its code go to ai/rules/.
Under 100 lines.
Only the init skill or a human changes this file: any other skill or agent reports the change it needs instead. -->

## Purpose

- Polix: engine that connects Claude Code and Codex to the workflow and rules of a repository through their hooks.
- Ships as one static binary per system (Linux, macOS, Windows; amd64, arm64), with the plugin `polix` (skills, agents).
- This repository is governed by its own polix: `polix.yaml`, `ai/rules/` and the hooks of `.claude/settings.json`.

## Stack

- **Language**: Go, with Cobra, Fang, Huh, Lip Gloss and yaml.v3.
- **Tooling**: just, golangci-lint and gofmt.
- **Install scripts**: sh (`install.sh`) and PowerShell (`install.ps1`).
- **CI**: GitHub Actions on Linux, macOS and Windows; releases published on GitHub Releases.

## Commands

- **Test**: `just test` (go vet and go test).
- **Single test**: `just test -run <TestName>`.
- **Build**: `just build` (into `build/polix`).
- **Install**: `just install` (go install, the binary the hooks run; rerun it after changing the engine).
- **Lint**: `golangci-lint run ./...`.
- **Format**: `gofmt -w .`.
- **Check**: `just check` (install, then `polix check`, `polix rules check` and `polix doctor --offline`).
- **Validate plugin**: `claude plugin validate plugins/polix`.
- **Release assets**: `just dist <version>` (into `dist/`).
- **Release**: `just release <version>` (tags a clean, pushed main and pushes the tag; CI publishes it).

## Layout

- **ai/**: instructions for the AI only; without it the repository is a plain project.
- **ai/plans/**: one plan per file, written by the `plan` skill.
- **ai/rules/**: rules on how to write the code, one `yaml` file per topic.
- **cmd/**: the entry point, `cmd/polix`.
- **docs/**: design, commands and limits (`polix.md`), recipes (`use-cases.md`) and assets.
- **examples/**: reference `polix.yaml`, hook settings for Claude Code and Codex, AGENTS.md fragment for governed repos.
- **internal/**: packages agent, cli, engine, gitx, glob, rules, setup, shell, state, update, version, workflow.
- **plugins/**: the plugin `polix` (skills, agents) and the Go package `plugins` that embeds it in the binary.
- **polix.yaml**: the workflow of the agents, enforced by polix through the hooks.
- **scripts/**: `dist.sh`, which builds the release assets.

## Testing

- **Unit**: Go testing, in `*_test.go` next to each package of internal/ and plugins/.
- **Install scripts**: Go tests in internal/cli/, also under PowerShell 5.1 and 7 on Windows CI.

## Rules

- The rules for the AI are in `ai/rules/*.yaml`.

## Documentation

- The documentation of the project is in docs/ (`polix.md`, `use-cases.md`), not published as a website.
