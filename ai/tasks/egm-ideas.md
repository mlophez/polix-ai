# Ideas from EGM

EGM (Elephant-Goldfish Model, https://egm.dev, repository `google/elephant-goldfish-model`, Apache-2.0) is a plugin
for Claude Code, Codex, Antigravity and other harnesses to design and review software with agents.
Its motto: "Design with an elephant. Review with a goldfish."
The elephant is the agent with the whole conversation, which drafts the design; the goldfish are subagents with a
fresh context that only read the written document, so they cannot validate the assumptions of the author.
Status when read (2026-11-12): about 18k stars, 198 commits, no published release; `docs/designs/architecture.md`
dated 2026-06-06.

## What EGM is

- Skills: `/egm-brainstorm` (brief, `concepts_brief.md`), `/egm-draft` (`design_draft.md`, then
  `docs/designs/<feature>.md`), `/egm-review`, `/egm-resolve` (go over the findings and agree changes),
  `/egm-review-loop` (pick reviewers and standards, for documents that are not software), `/egm-bootstrap`
  (document existing code) and `/egm-drift-report` (compare the designs with the code).
- `/egm-review` runs five stages: Zoom Out (simplicity, YAGNI, reuse), Comprehension Test (a naive reader audits that
  the document stands alone), Critic Review (design, bottlenecks, failure modes), a second Zoom Out over what stages
  2 and 3 added, and Implementation Readiness (paths, function signatures, test cases).
- A Synthesizer fans out the reviewers in parallel, deduplicates their findings and writes one Markdown report.
- Reviewers are stateless prompts in `agents/`; rubrics are versioned files in `resources/document_standards/`
  (`zoom-out-standard.md`, `feature-spec-standard.md`, `implementation-readiness-standard.md`).
- Reviews need fresh subagents: without them, EGM stops the review.
- Artifact-first: long outputs go to files, never to the chat.
- Write boundaries: the design phases only write specification and review artifacts, and the implementation is
  bounded by the "Target Scope & File Boundary" list of the plan. Its own docs say: "This is a convention, and
  nothing enforces it."
- No hooks found in the pages read.

## How it relates to polix

- It overlaps with `plan`, `build`, `review` and `polix:reviewer`; `plan` already aims for a plan that one executor
  runs alone knowing only the plan and the code, which is the goldfish idea.
- The difference: EGM is a method, polix enforces through hooks. What EGM leaves as convention, polix can make a gate.
- So the point is not to install or vendor EGM, but to take some of its ideas.

## Changes, by value

- [ ] 1. Review the plan before building it. Today `polix:reviewer` reviews the code against the plan, but nothing
      reviews the plan itself. Add a zero-context review of the plan file alone, with the three most useful EGM
      criteria: Zoom Out, Comprehension and Implementation Readiness. In `polix.yaml`, a gate: `build` does not start
      until the plan has an approved review. It targets the most common cause of a bad build, an ambiguous plan.
- [ ] 2. Enforce write boundaries. With `PreToolUse`: while planning, only the plan file is writable; while
      building, only the files the plan declares, and going outside them needs a justification. First check whether
      `plugins/polix/skills/plan/template.md` already lists the files a plan touches, or add that section.
- [ ] 3. Drift between `docs/` and the code: a focus of `audit`, or a warning in `run_on_stop` when a change touches
      code described in `docs/polix.md` or `docs/architecture.md`.
- [ ] 4. A recipe in `docs/use-cases.md` for repositories that already use EGM: a `polix.yaml` that requires a report
      of `/egm-review` before implementing. Polix brings the enforcement, EGM the method.

Recommended: 1 and 2 first.

## Notes

- Conflict with `ai/tasks/reviewer-latency.md`: change 1 adds one more review round. It is offset if the plan
  reviewer runs at `effort: medium` and reads only the plan, which is short.
- License: Apache-2.0 allows reusing their rubrics, but copied text must keep the attribution and the NOTICE.
  Prefer rewriting the rubrics in the style of this repository.
- Read on 2026-11-12: https://egm.dev/workflow and the pages of `egm-review`, `egm-resolve` and `egm-review-loop`,
  `docs/harnesses.md`, `docs/faq.md`, `TESTING.md`, every rubric of `resources/document_standards/` and every
  reviewer of `agents/`. Not read: the `SKILL.md` files of their skills. Summary in `What EGM reviews`, paraphrased.
- No `NOTICE` file seen at the root of their repository (the listing read may be partial); each reviewer file carries
  `license: Apache-2.0` in its frontmatter.

## What EGM reviews

### Rubrics

The rubrics are pass or fail bars with no severity nor output format: the reviewer that applies one sets both.

- **Zoom out**: the objective and its why are clear, or the review stops and asks for context; the plan attacks the
  root problem, not a symptom; it is simple (no YAGNI, no premature optimization, no dead end); it stays at the
  right altitude and layer, with structural fixes over workarounds; it reuses the code, patterns and platform there.
- **Feature spec**: a new engineer understands the feature and its trade-offs; alternatives are mandatory, each with
  a concrete reason to reject it; risks, failure modes, security and scale have mitigations; it has problem, goals and
  non-goals, architecture, alternatives and risks.
- **Implementation readiness**: a new engineer builds it right the first time; zero decisions left to the implementer
  (schemas, paths, dependencies, edge cases); each phase builds and tests; a target scope of exact repository paths,
  globs only for generated output, usable as a write allowlist; actionable steps per file; tests of the happy path
  and the edge cases.
- **Re-implementation**: zero assumptions about defaults, errors, patterns or output schemas; external contracts,
  behaviour and limits in, syntax and internal layout out.
- **Heilmeier**: the eight questions of the Heilmeier catechism, each answered without ambiguity.

### Reviewers

Each one is read-only, gets no conversation, only the target file, its range and the rubric, and answers without
chitchat.
Every finding has a severity, `Blocker`, `Major` or `Nit`, the scope it affects, the problem and its rationale or fix.

- **Comprehension** (naive reader): paraphrases what the document proposes and how the current system works per the
  document; a section it cannot paraphrase is a `Blocker`; it flags project terms never defined, broken links and
  assumed prerequisites, after scanning the whole document.
- **Design critic**: the questions of the rubric, unjustified assumptions, failure modes, scale and security left
  without mitigation.
- **Zoom out** (skeptical architect): applies the zoom-out rubric and searches the code whenever the document proposes
  a new utility, to find an existing one.
- **Implementation readiness** (tech lead): whether the reader knows what to code, where and how; every choice left to
  the implementer is a finding.
- **Structural**, **style** and **inventive catalyst** (TRIZ, false compromises): less useful for plans of polix.
- **Synthesizer**: launches the reviewers in parallel, optionally two instances each for recall, retries a failed one
  once and then reports it `not run`, never as zero findings; deduplicates, groups by severity and writes one report.

### Workflow

- Stages: approach, clarity for a new reader, design decisions, approach again over what the earlier stages added,
  and whether someone can build from it; then resolve; a person approves before the implementation.
- The review loop ends after two rounds in a row with no serious finding, or after five, then asks the user.
- Resolving runs in the conversation that wrote the document: findings that need the user come first, and the user
  accepts, rejects or questions each one.
- Without a subagent tool the review stops instead of running in the same context; the Claude adapters hide
  `CLAUDE.md` from the reviewers, and Codex leaked `AGENTS.md` to them.
- Cost: a review spends more than a normal conversation; they name no model nor effort for the reviewers, and they
  test the prompts only for format and isolation, never for the quality of the findings.

### What it changes in change 1

- The plan reviewer is one subagent with three lenses, comprehension, zoom out and implementation readiness, not a
  synthesizer with three subagents: the plan is short, and `reviewer-latency.md` asks for fewer rounds.
- Most of implementation readiness is already in the self-review of `plan` (Step 7), but judged by its author: the
  value is the same checks from a fresh context, plus the paraphrase of comprehension, which the self-review lacks.
- Zoom out needs the code: the reviewer reads the plan and searches the code, never the conversation.
- `Files:` of each task already is the target scope; readiness checks that it lists exact paths, which change 2 needs.
- Severities map to those of `polix:reviewer` (`blocking`, `major`, `minor`) to keep one vocabulary.
- Open for the plan: who resolves the findings (the session of `plan`, with the user) and where the approval is
  recorded so `build` can check it (a line of the header of the plan, for example).
