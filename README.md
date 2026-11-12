<div align="center">

# Polix

**Declarative guardrails for Claude Code: one `polix.yaml` decides what the agent may not touch, what must pass and
what runs at every step.**

One static binary that works with Claude Code through its hooks; Codex support is not implemented.

[![CI](https://github.com/mlophez/polix-ai/actions/workflows/ci.yml/badge.svg)](https://github.com/mlophez/polix-ai/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/mlophez/polix-ai?sort=semver)](https://github.com/mlophez/polix-ai/releases/latest)
[![Go](https://img.shields.io/github/go-mod/go-version/mlophez/polix-ai)](go.mod)
[![License](https://img.shields.io/github/license/mlophez/polix-ai)](LICENSE)
![Platforms](https://img.shields.io/badge/platforms-linux%20%7C%20macOS%20%7C%20windows-informational)

[Highlights](#highlights) ·
[Quick start](#quick-start) ·
[Engine and harness](#the-engine-and-the-harness) ·
[Docs](#learn-more) ·
[Use cases](docs/use-cases.md) ·
[Status](#status) ·
[Changelog](CHANGELOG.md)

</div>

---

<p align="center">
  <img src="docs/assets/overview.svg" alt="Claude Code calls Polix through its hooks, and git on every commit (Codex is not implemented); Polix reads polix.yaml and ai/rules/, keeps what it observed and decided in log.jsonl, and answers by blocking, injecting context or allowing the action" width="100%">
</p>

## The problem

You wrote it down in `AGENTS.md`: *run the tests before you commit, get the diff reviewed, never push to main.*
The agent agrees, and then, forty tool calls later, it forgets.

Instructions in a prompt are suggestions. Hooks can enforce them, but a pile of shell scripts per repository gets
fragile quickly. Each one parses JSON with `jq`, has no memory between calls, breaks on Windows and drifts away from
the copy in the next repository.

## The idea

**Polix** gives you one declarative file, `polix.yaml`, that says what must happen on each event of an agent
session: what is blocked, what runs and what is injected into the context. The same binary answers every hook in
milliseconds and keeps an append-only log of what it observed and decided.

```yaml
version: 1

guardrails:
  protected_files: [AGENTS.md, CLAUDE.md, .env]      # no agent writes them, the main session included
  forbidden_commands:                                 # the reason goes back to the agent
    - pattern: "git push*--force*"
      reason: "Force pushes rewrite shared history: ask a person to run it."
  protected_branches: [main]                          # nobody commits there
  format_on_edit:                                     # right after each edit
    - { run: "golangci-lint fmt {path}", path: "*.go" }
  run_on_stop:                                        # the agent cannot end its turn while one fails
    - { run: "go test ./...", changed: "*.go" }

on:
  pre_commit:
    - unless: { finished: reviewer }                  # the reviewer subagent saw the diff as it is now
      action: { block: "Review the diff with the reviewer subagent, then commit again." }

githooks:
  commit_msg:                                         # every commit, also those a person makes
    - unless: { message: "^[A-Z]+-[0-9]+: " }
      action: { block: "Start the message with the key of the ticket, as PLAT-123: ..." }
```

`polix.yaml` has three sections. `guardrails:` holds the basics every project wants from the first minute. `on:` is
where your team orchestrates its flow: reviews, plans, tickets, each an entry that says on which event it applies,
when, and what it does. `githooks:` carries gates to the hooks of git, so they hold for every commit, whoever makes
it.

Here it is in a real Claude Code session asked to *add a Reverse function with a test and commit it*
([`docs/assets/demo/polix.yaml`](docs/assets/demo/polix.yaml)). Polix blocks the commit on `main` and then the commit
without a review, and Claude recovers by itself: a branch, the reviewer, the tests Polix runs, the commit.

<p align="center">
  <img src="docs/assets/demo.gif" alt="A Claude Code session adds a Reverse function and its test; Polix blocks the commit on main, Claude creates a branch, Polix blocks the commit without a review, Claude runs the reviewer subagent and then commits" width="100%">
</p>

> The human writes the rules. Polix only enforces them.
> It **measures and never judges**: conditions are facts about the repository, the event and the state.
> Judging the code itself is left to a reviewer subagent.

## Highlights

- **Tied to content, not to time.** A check Polix ran green is not run again on the same content, and
  `finished: reviewer` holds until a file the reviewer looked at changes. Edit one line and they apply again; nothing
  to invalidate by hand. A `go test` the agent ran by itself never counts: Polix only trusts what it ran.
- **Gates, reminders and notices.** A `block` holds every time until its `unless` does, `once: true` turns it into a
  reminder once per content, an `inject` warns the AI and a `notify` tells the person, each one line of YAML.
- **Gates of git for every commit, not only the agent's.** `githooks:` blocks or runs a check on `pre_commit` and
  `commit_msg`, by branch, by the staged files or by the message, so a commit format or a test gate binds the people
  of the team too. No hook scripts to copy: a session of the AI installs them in `.git/config` when `polix.yaml`
  declares them, and `polix install --githooks` does it for anyone else.
- **Rules that reach the agent and CI.** Rules in `ai/rules/` have a scope: Polix injects only the ones that match
  the file being edited, and `polix rules check --changed` runs the same checks in CI.
- **Validated and safe by default.** `polix config check` rejects unknown keys, conditions and placeholders, so a
  typo cannot quietly switch a guardrail off. A broken `polix.yaml` fails closed for edits and commands, and a loop
  guard never traps a stop.
- **Correct where shell scripts are not.** Command lines are split as a shell runs them, quotes and pipelines
  included, so `cd x && just test | tail -3` is three commands and `git commit` is found inside any of them.
- **An audit trail.** Every decision is logged with the entry that caused it, next to the checks that passed and the
  subagents that finished on each content.
- **One binary, everywhere.** Linux, macOS and Windows (amd64 and arm64), tested on all three in CI, the same under
  `sh`, Git Bash and PowerShell. About 13 ms per hook call, and it updates itself, checksum-verified.

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

The installers verify the binary against `checksums.txt` and install the plugin `polix` for you.

**2. Register the hooks of the repository**

```bash
polix install    # the hooks of Claude Code in .claude/settings.json and the hooks of git in .git/config
```

Commit `.claude/settings.json` so the team gets the hooks; each part has a flag of its own (`--skills`, `--hooks`,
`--githooks`).

**3. Write your workflow and check it**

Start from the example [above](#the-idea) or from [`examples/polix.yaml`](examples/polix.yaml), which annotates
every option, then:

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

**The harness** is optional: the plugin `polix` ships a development flow ready to use, with `/polix:plan`,
`/polix:build` (run by `polix:builder`, reviewed by `polix:reviewer`), `/polix:review` and `/polix:audit`, plus the
skills that set Polix up (`/polix:init`, `/polix:workflow`, `/polix:add-rule`, `/polix:doctor`, `/polix:risk`). A
skill runs only when you or your `polix.yaml` ask for it; Polix enforces a phase when your workflow says so:

```yaml
on:
  pre_commit:
    - unless: { finished: "polix:reviewer" }
      action: { block: "Review the diff with /polix:review before committing" }
```

## Learn more

- [docs/use-cases.md](docs/use-cases.md): ready-to-paste recipes, from protected paths and tests that run themselves
  to a review before every commit, a plan before the first edit, and production kept out of reach.
- [docs/reference.md](docs/reference.md): every key, event, condition and verb of `polix.yaml`.
- [docs/polix.md](docs/polix.md): how a hook call is answered, the state, the gates of git, install and update, every
  command, and the known limits.
- [docs/architecture.md](docs/architecture.md) and [CONTRIBUTING.md](CONTRIBUTING.md): the layout of the code and
  how to build, test and check a change (`just test`, `just check`).

This repository is governed by its own Polix: [`polix.yaml`](polix.yaml), [`ai/rules/`](ai/rules) and the hooks in
[`.claude/settings.json`](.claude/settings.json).

## Status

Polix is 0.x: `polix.yaml` is `version: 1` and its format may still change; `requires:` lets a repository pin the
oldest Polix that knows every option it uses. What it does not do yet:

- Codex is not implemented: an adapter and an opt-in install (`--agent codex`) exist, never tested against a running
  Codex.
- The harness has no document phase yet.
- The log grows without bound and is appended without a lock.
- Releases are verified with checksums but are not signed yet.

The full list is in [docs/polix.md](docs/polix.md#known-limits), and what each release changes is in
[CHANGELOG.md](CHANGELOG.md).

## License

[Apache 2.0](LICENSE)
