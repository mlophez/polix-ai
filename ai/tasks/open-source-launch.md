# Open source launch of polix

Goal: get polix adopted outside Logalty as a deterministic harness for AI agents, starting with Claude Code.
The message and a demo that convinces in under a minute matter more than the channels; do the steps in order.

## 1. Before announcing

- [ ] Confirm with Logalty that polix can be published under `mlophez` and with which license; consider
      publishing it as a Logalty project instead, as technical branding.
- [x] Add a license: Apache-2.0.
- [x] Drop "proof of concept" from `docs/polix.md` and anywhere else it shows.
- [ ] Check that the name "polix" does not clash with a known project (GitHub, npm, PyPI) and use the same
      names everywhere (repo `polix-ai`, binary `polix`).

## 2. Positioning

- [x] Fix one sentence and repeat it everywhere (README, root help, `docs/polix.md`, GitHub description):
      "A deterministic harness for AI agents: one `polix.yaml` decides what the agent may not touch, what must pass
      and what runs at every step."
- [x] The problem it answers: the agent skips the rules (commits without tests, edits where it must not,
      ignores `AGENTS.md`).
- [x] The differentials to repeat:
  - Deterministic control of the agent to build a harness on, not audit or monitoring (unlike Gryph).
  - It measures facts, it does not ask another LLM to judge.
  - One static binary, no runtime, milliseconds per hook.
  - One file for several agents: only once Codex is supported.

## 3. README that converts

- [x] Positioning sentence at the top.
- [x] GIF of a real session: the agent tries to commit without the reviewer, polix denies it, the agent fixes it
      by itself.
- [x] One-line install, plus `go install`.
- [x] An example `polix.yaml`: the full commented one, folded, instead of a minimal one.
- [x] A link to the recipes of `docs/use-cases.md`.
- [x] An honest comparison with `CLAUDE.md`, the permissions of `settings.json` and hand-written hooks.
- [x] Social preview image, topics and description of the repository.

## 4. Channels, in this order

- [ ] Show HN on Tuesday 2026-11-17, 15:00 to 17:00 Madrid time, answering comments the first hours; the post and
      the first comment are drafted.
- [ ] awesome-claude-code from 2026-11-18, through its web issue form only (no PRs); the fields are drafted.
- [ ] Claude Code communities a few days after Show HN: r/ClaudeAI, r/ClaudeCode, r/golang, the Anthropic
      Discord, and the plugin marketplace if there is one.
- [ ] X, Bluesky and LinkedIn: short threads with the GIF, tagging people who write on agentic coding.
- [ ] Local community: a 15 minute talk at a Madrid meetup (Go, AI, DevOps).
- [ ] Codex communities (r/ChatGPTCoding, r/OpenAI): only once Codex is supported and tested.

## 5. Content

- [ ] Post: "My agents skipped the rules; this is how I forced them with hooks", a real case with before and
      after.
- [ ] Technical post: how to detect a `git commit` reliably from a hook and why polix requires it unchained.
- [ ] A gallery of ready-to-copy `polix.yaml`: starters by stack (issue #6), review before PR, no edits to
      migrations, rules injected per folder.

## 6. From visits to users

- [x] `good first issue` and `help wanted` issues (#3 to #8) and a `CONTRIBUTING.md` for contributors.
- [x] Community files: `SECURITY.md`, `CODE_OF_CONDUCT.md`, issue forms, pull request template, Discussions and
      private vulnerability reporting.
- [ ] Frequent releases with a readable changelog, announced with a short thread each.
- [ ] Fast answers to the first issues: the first 20 users decide the word of mouth.
- [ ] An example repository, ready to clone, already governed by polix.
- [ ] A contact address for the code of conduct other than the GitHub profile.

## Next step

- [x] Rewrite the README with the structure of section 3, then draft the Show HN post.
- [ ] Draft the launch thread for X, Bluesky and LinkedIn.
