<div align="center">

# Polix

**A deterministic harness for AI agents.**<br>
**One `polix.yaml` decides what the agent may not touch, what must pass and what runs at every step.**

Works with Claude Code today: one static binary, about 13 ms per hook call, on Linux, macOS and Windows.

[![CI](https://github.com/mlophez/polix-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/mlophez/polix-ai/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mlophez/polix-ai?sort=semver)](https://github.com/mlophez/polix-ai/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/mlophez/polix-ai)](go.mod)
[![License](https://img.shields.io/github/license/mlophez/polix-ai)](LICENSE)
![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20macOS%20%7C%20windows-informational)

[Highlights](#highlights) ·
[Why Polix](#why-not-just) ·
[Quick start](#quick-start) ·
[Engine and harness](#the-engine-and-the-harness) ·
[Docs](#learn-more) ·
[Use cases](docs/use-cases.md) ·
[Status](#status) ·
[Changelog](CHANGELOG.md)

</div>

---

<p align="center">
  <img src="docs/assets/overview.svg" alt="Claude Code calls Polix through its hooks, and git on every commit (Codex is not implemented); Polix reads polix.yaml, keeps what it observed and decided in one log per session of the agent, and answers by blocking, injecting context or allowing the action" width="100%">
</p>

<details>
<summary><b>See it in a real session</b>: Claude Code asked to <i>add a Reverse function with a test and commit
it</i></summary>

<br>

Polix blocks the commit on `main`, then the commit without a review, and Claude recovers on its own: it creates a
branch, runs the reviewer, lets Polix run the tests and commits
([`docs/assets/demo/polix.yaml`](docs/assets/demo/polix.yaml)).

<p align="center">
  <img src="docs/assets/demo.gif" alt="A Claude Code session adds a Reverse function and its test; Polix blocks the commit on main, Claude creates a branch, Polix blocks the commit without a review, Claude runs the reviewer subagent and then commits" width="100%">
</p>

</details>

## The problem

You wrote it down in `AGENTS.md`: *run the tests before you commit, get the diff reviewed, never push to main.*
The agent agrees, and then, forty tool calls later, it forgets.

Instructions in a prompt are suggestions. Hooks can enforce them, but a pile of shell scripts per repository gets
fragile quickly: each one parses JSON with `jq`, has no memory between calls, breaks on Windows and drifts away from
the copy in the next repository.

## The idea

**Polix** gives you one declarative file, `polix.yaml`, that says what must happen on each event of an agent
session: what is blocked, what runs and what is injected into the context. The same binary answers every hook in
milliseconds and keeps an append-only log of what it observed and decided.

`polix.yaml` has three sections. `guardrails:` holds the basics every project wants from the first minute. `on:` is
where your team orchestrates its flow: reviews, plans, tickets, each one an entry that says which event it applies
to, when, and what it does. `githooks:` turns the same gates into git hooks, so they hold for every commit, whoever
makes it.

> The human writes the rules. Polix only enforces them.
> It **measures and never judges**: conditions are facts about the repository, the event and the state.
> Judging the code itself is left to a reviewer subagent.

## Highlights

- **Tied to content.** A green check or a finished review holds until the files change; edit one line and the gate
  applies again.
- **Gates, reminders and notices.** `block`, `once: true`, `inject` and `notify`, each one line of YAML.
- **Every commit, not only the agent's.** `githooks:` gates commits by branch, staged files or message, including
  the ones people make.
- **Rules for the agent and for CI.** Rules in `ai/rules/` reach the agent for the file it edits, and CI through
  `polix rules check`.
- **Safe by default.** A typo is rejected, a broken config fails closed and a loop guard never traps a stop.
- **An audit trail.** Every decision is logged with the entry that caused it.
- **One binary, everywhere.** Linux, macOS and Windows, about 13 ms per hook call, and it updates itself.

## Why not just...

**...write it in `CLAUDE.md` or `AGENTS.md`?** Instructions compete with everything else in the context window and
fade over a long session. Polix keeps them in your `AGENTS.md` as guidance, and turns the ones that must hold into
gates the agent cannot skip.

**...use the permissions of `.claude/settings.json`?** Permissions allow or deny a tool call by its pattern, and that
is all. They cannot say "no commit until the tests pass", "only once the reviewer has seen this diff" or "remind me
once per change". Polix adds conditions on the repository, the event and the session state.

**...write hooks by hand?** You can, and Polix runs on the same hooks. What it replaces is the glue: one script per
check, JSON parsed with `jq`, no memory of what already ran, a copy per repository and scripts that break on
Windows. With Polix, each gate is a few lines of YAML that are validated, formatted, logged and work the same on
every system.

## Quick start

**1. Install the binary and the plugin**

```bash
# macOS and Linux (also Windows with Git Bash)
curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh
```

```powershell
# Windows (PowerShell)
powershell -NoProfile -ExecutionPolicy Bypass -Command "irm https://github.com/mlophez/polix-ai/releases/latest/download/install.ps1 | iex"
```

The installers verify the binary against `checksums.txt` and install the `polix` plugin for you.

With Go, on any system:

```bash
go install github.com/mlophez/polix-ai/cmd/polix@latest
polix install --skills    # the polix plugin for your user
```

**2. Register the hooks of the repository**

```bash
polix install    # Claude Code hooks in .claude/settings.json and git hooks in .git/config
```

Commit `.claude/settings.json` so the whole team gets the hooks. Each part has its own flag (`--skills`, `--hooks`,
`--githooks`).

**3. Write your workflow and check it**

Save a `polix.yaml` at the root of the repository. The example below shows most of what Polix can do; start from it,
or from [`examples/polix.yaml`](examples/polix.yaml), which annotates every option.

<details>
<summary><b>Example <code>polix.yaml</code></b>: protected files, forbidden commands, format on edit, tests before
stopping, a review before large commits and a ticket key in every commit message</summary>

```yaml
version: 1

# The programs the repository needs on the PATH: the first prompt of a session tells the AI which are missing.
requirements:
  - go
  - golangci-lint

guardrails:
  # No agent writes these files, the main session included.
  protected_files:
    - AGENTS.md
    - CLAUDE.md
    - .env

  # No agent runs these commands; the reason goes back to the agent.
  forbidden_commands:
    - pattern: "*rm -rf /*"

    - pattern: "git push*--force*"
      reason: "Force pushes rewrite shared history: ask a person to run it."

  # The agent commits on none of these branches.
  protected_branches:
    - main

  # Each Go file is formatted right after the agent edits it.
  format_on_edit:
    - run: golangci-lint fmt {path}
      path: "*.go"

  # The agent cannot end its turn while the tests fail; they run only when Go code changed.
  run_on_stop:
    - run: go test ./...
      changed: "*.go"

on:
  pre_commit:
    # A change of Go code of 50 lines or more needs the reviewer to have seen the diff as it is now:
    # editing a line after the review asks for a new one.
    - when: { changed: ["*.go", go.mod], changed_lines: 50 }
      unless: { finished: polix:reviewer }
      action:
        block: "Review the diff with the polix:reviewer subagent, then commit again."

githooks:
  commit_msg:
    # Git runs it on every commit, also those a person makes: the message starts with the ticket key.
    - unless: { message: ["^[A-Z][A-Z0-9]+-[0-9]+: ", "^Merge "] }
      action:
        block: "Start the message with the ticket key, as PLAT-123: {message}"
```

</details>

Then check it:

```bash
polix config check     # validate polix.yaml and ai/rules/
polix doctor           # check that this machine and repository are ready
polix graph            # draw the workflow as a Mermaid diagram
```

Ask the agent in a new session to force push: Polix blocks the command and the agent reads the reason.

**4. Tell the agent**

Paste [`examples/AGENTS-polix.md`](examples/AGENTS-polix.md) into your `AGENTS.md` or `CLAUDE.md`.

## The engine and the harness

**The engine** is the binary, `polix.yaml` and `ai/rules/`. It enforces whatever process you write down and assumes
none: use it with your own skills, agents and conventions.

**The harness** is optional: the `polix` plugin ships a ready-to-use development flow, with `/polix:plan`,
`/polix:build` (run by `polix:builder`, checked against the plan by `polix:verifier` and reviewed by
`polix:reviewer`), `/polix:review` and `/polix:audit`, with `/polix:task` to add tasks to the backlog, plus the
skills that set Polix up (`/polix:init`, `/polix:workflow`, `/polix:add-rule`, `/polix:doctor`, `/polix:risk`). A
skill of the flow runs only when you type it, and Polix enforces a phase only when your workflow says so:

```yaml
on:
  pre_commit:
    - unless: { finished: "polix:reviewer" }
      action:
        block: "Review the diff with the polix:reviewer subagent before committing"
```

## Learn more

- [docs/use-cases.md](docs/use-cases.md): ready-to-paste recipes, from protected paths and tests that run themselves
  to a review before every commit, a plan before the first edit, and production kept out of reach.
- [docs/reference.md](docs/reference.md): every key, event, condition and verb of `polix.yaml`.
- [docs/polix.md](docs/polix.md): how a hook call is answered, the state, the git gates, install and update, every
  command, and the known limits.
- [docs/architecture.md](docs/architecture.md) and [CONTRIBUTING.md](CONTRIBUTING.md): the layout of the code and
  how to build, test and check a change (`just test`, `just check`).

This repository is governed by its own Polix: [`polix.yaml`](polix.yaml), [`ai/rules/`](ai/rules) and the hooks in
[`.claude/settings.json`](.claude/settings.json).

## Status

Polix is 0.x: `polix.yaml` is `version: 1` and its format may still change; `requires:` lets a repository pin the
oldest Polix version that supports every option it uses. What it does not do yet:

- Codex is not supported: an adapter and an opt-in install (`--agent codex`) exist, but they have never been tested
  against a running Codex.
- The harness has no documentation phase yet.
- The log grows without bound and is appended without a lock.
- Releases are verified with checksums but are not signed yet.

The full list is in [docs/polix.md](docs/polix.md#known-limits), and what each release changes is in
[CHANGELOG.md](CHANGELOG.md).

## License

[Apache 2.0](LICENSE)
