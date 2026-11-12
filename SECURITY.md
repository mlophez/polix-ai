# Security policy

Polix sits between an AI agent and your repository: it decides what the agent may run, write and commit. A way to
get past a gate it should hold is a security issue, and so is anything that lets `polix.yaml`, a hook payload or a
release asset run code it should not.

## Supported versions

Polix is 0.x: only the latest release gets fixes. `polix update` moves to it.

## Reporting a vulnerability

Do not open a public issue. Report it privately through
[GitHub security advisories](https://github.com/mlophez/polix-ai/security/advisories/new), with:

- the version (`polix --version`) and the system;
- the smallest `polix.yaml` and the steps that reproduce it;
- what you expected Polix to do and what it did, with the lines of `polix log` if they help.

Reports are handled on a best-effort basis: you get an answer within a week, and a fix ships as a new release, with
credit in the changelog unless you prefer otherwise.

## Scope

In scope:

- a gate of `guardrails:`, `on:` or `githooks:` that an agent can bypass while the hooks are installed;
- a broken or unreadable `polix.yaml` that does not fail closed;
- command injection through a hook payload, a path, a commit message or a template value;
- the installers (`install.sh`, `install.ps1`) and `polix update` installing an asset that does not match
  `checksums.txt`.

Out of scope, documented in [docs/polix.md](docs/polix.md#known-limits):

- a person or an agent that removes the hooks or edits `.claude/settings.json` when the workflow does not protect
  it;
- releases that are checksummed but not signed yet.
