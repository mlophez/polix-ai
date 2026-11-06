# Open source launch of polix

Goal: get polix adopted outside Logalty as the declarative guardrails for Claude Code and Codex.
The message and a demo that convinces in under a minute matter more than the channels; do the steps in order.

## 1. Before announcing

- [ ] Confirm with Logalty that polix can be published under `mlophez` and with which license; consider
      publishing it as a Logalty project instead, as technical branding.
- [ ] Add a license: MIT or Apache-2.0 (Apache-2.0 adds a patent grant, valued by companies).
- [ ] Drop "proof of concept" from `docs/polix.md` and anywhere else it shows.
- [ ] Check that the name "polix" does not clash with a known project (GitHub, npm, PyPI) and use the same
      names everywhere (repo `polix-ai`, binary `polix`).

## 2. Positioning

- [ ] Fix one sentence and repeat it everywhere, for example:
      "Declarative guardrails for Claude Code and Codex: one `polix.yaml` decides what is blocked, required and
      run at every step of the agent."
- [ ] The problem it answers: the agent skips the rules (commits without tests, edits where it must not,
      ignores `AGENTS.md`).
- [ ] The differentials to repeat:
  - Deterministic: it measures facts, it does not ask another LLM to judge.
  - One file for several agents.
  - One static binary, no runtime, milliseconds per hook.

## 3. README that converts

- [ ] Positioning sentence at the top.
- [ ] GIF or asciinema of 20-30 s: the agent tries to commit without the reviewer, polix denies it, the agent
      fixes it by itself.
- [ ] One-line install (already exists).
- [ ] A minimal `polix.yaml` of about 10 lines solving a typical case.
- [ ] A "Recipes" section linking to `docs/use-cases.md`.
- [ ] An honest comparison with hand-written hooks or only `CLAUDE.md`, and why they are not enough.

## 4. Channels, in this order

- [ ] Claude Code communities: r/ClaudeAI, the Anthropic Discord, PRs to the "awesome-claude-code" lists, and
      the plugin marketplace if there is one.
- [ ] Codex communities: r/ChatGPTCoding, r/OpenAI, Codex forums and discussions; multi-agent is the argument.
- [ ] Show HN, only once the README and the demo are polished: "Show HN: Polix – declarative guardrails for
      Claude Code and Codex agents", Tuesday to Thursday, US morning, answering comments the first hours.
- [ ] X, Bluesky and LinkedIn: short threads with the GIF, tagging people who write on agentic coding and the
      devrel of Anthropic and OpenAI.
- [ ] Local community: a 15 minute talk at a Madrid meetup (Go, AI, DevOps).

## 5. Content

- [ ] Post: "My agents skipped the rules; this is how I forced them with hooks", a real case with before and
      after.
- [ ] Technical post: how to detect a `git commit` reliably from a hook and why polix requires it unchained.
- [ ] A gallery of ready-to-copy `polix.yaml`: tests before commit, review before PR, no edits to migrations,
      rules injected per folder.

## 6. From visits to users

- [ ] "good first issue" labels and a short `CONTRIBUTING.md`.
- [ ] Frequent releases with a readable changelog.
- [ ] Fast answers to the first issues: the first 20 users decide the word of mouth.
- [ ] An example repository, ready to clone, already governed by polix.

## Next step

- [ ] Rewrite the README with the structure of section 3, then draft the Show HN post and the launch thread.
