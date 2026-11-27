---
name: improving-agent-context
description: >
  Improves the context an agent receives in each phase of the flow, from the repository and from outside it: it
  detects duplicates, contradictions, inconsistencies and misplaced facts, and rewrites the sources the user confirms.
  It runs only when the user asks for it.
  It never condenses text no finding touches: that is the skill synthesize.
user-invocable: true
disable-model-invocation: true
argument-hint: "[file or folder to fix, optionally against places, e.g. ai/ctx against AGENTS.md; empty for all]"
---

# Improving the context of an agent

Map what the agent receives in each phase of the flow and cross it, within a phase and across phases.
Rewrite the repository so each fact is stated once, in its home and the same way in every phase, as the user confirms.
Touch only the sentences of a finding: the rest of each file keeps its words.

## Requirements

- A git repository, since Step 1 lists its sources with `git ls-files`.
  Without it, stop and tell the user.

## Input

- **Target**: the path of a file or folder of the repository to fix, from the arguments or the conversation.
  Without it, the target is every source of the repository (see `Sources`).
- **Against**: the places to cross the target with, the paths after `against` in the arguments or the conversation.
  Without them, the places are every source of the phases the target reaches (see `Phases`).

## Output

- **Phases**: each phase, with the sources it receives, one line each.
- **Conflicts**: each pair of sources of one phase that contradict each other, or "none".
  Give both lines quoted, the wording written and the mark `external` when it applies (see Step 5).
- **Inconsistencies**: each fact one phase states otherwise than another, or "none".
  Give both lines, both phases, the wording written and the mark `external` when it applies.
- **Duplicates**: each fact cut, merged or replaced with a pointer, or "none".
  Give the file and line it left, the source that keeps it and its marks `on demand` and `external` when they apply.
- **Misplaced**: each fact outside the source of its kind, or "none".
  Give its line, its home (see `Homes`) and whether it moved.
- **Kept**: each finding left as it was, with the reason, or "none".
- **Files**: each file edited, with its lines before and after.
- **Pending**: the edits a hook denied, with their diff, and the checks that failed or were skipped, or "none".

## Limits

- Only edit the files of the target and the home a merge or a move writes to, and never stage nor commit them.
- Never edit a source outside the repository, nor cut a copy because one holds it: it belongs to one user.
- Edit no file until the user confirms the diff of Step 8.
- Cut an `on demand` duplicate, or replace it with a pointer, only when the user confirms it with its mark in sight.
- Rewrite only the sentences a finding touches; condensing the rest of a file is the skill `synthesize`.
- Never add a fact no source states, nor pick the side of a contradiction without evidence: leave it to the user.
- Give every finding both sources with their lines, or the context of this session quoted.
- When unsure whether two sources say the same, keep both and list the pair in `Kept`.

## How to operate

Copy this checklist and track your progress:

```text
Improve progress:
- [ ] Step 1: List the sources of the repository and from outside it (see `Sources`).
- [ ] Step 2: Map the phases and the sources each one receives (see `Phases`).
- [ ] Step 3: Read the sources of the target, of the phases it reaches and of `Against` (see Step 3).
- [ ] Step 4: Cross the facts within each phase and across phases (see Step 4).
- [ ] Step 5: Classify each finding and decide its fix (see Step 5).
- [ ] Step 6: Search with `rg` for links to the headings of each file to edit and for tests or tools that read it.
- [ ] Step 7: Apply the fixes to copies of the files in a temporary folder and verify them (see Step 7).
- [ ] Step 8: Show the findings and the diff, and wait for the confirmation (see Step 8).
- [ ] Step 9: Write the confirmed files over the originals, run the checks and report (see Step 9).
```

### Step 3: Read the sources of the target, of the phases it reaches and of `Against`

- Read every source of the target whole.
- With at most three phases the target reaches, or without subagents, as in Codex, read every other source whole too.
- With more, leave the other sources to the subagents of Step 4.

### Step 4: Cross the facts within each phase and across phases

1. With more than three phases the target reaches and subagents available, split the phases in at most eight groups.
2. Delegate each group to a `general-purpose` subagent, all in one message.
   Give each one its phases, the paths of the sources of each and the context of this session it receives quoted.
   Pass it this prompt:
   ```text
   Each phase given is one moment of the flow of an agent, with the sources it receives together.
   Read every source given whole.
   For each phase, list every fact two of its sources state, by meaning and not by wording, with both files and lines.
   Classify each one as identical, partial (quote what one adds) or contradicting.
   Answer "none" for a phase where no two sources share a fact.
   For each source, also list every format, name, path, command, number and condition it states, with its line.
   ```
3. Check each pair and fact a subagent reports against the lines it quotes, and drop the ones they do not support.
4. List each instruction, fact, condition, exception, number, name, path, command and link of each source you read.
5. Cross them with each other and with the facts the subagents listed, to find the pairs across phases.
6. Find the sources that state each one by its meaning; use `rg` only to locate its names, paths and commands.

### Step 5: Classify each finding and decide its fix

1. Classify each pair and decide its fix:
   - **Identical**: two sources of one phase state it, and the copy says nothing the other lacks: cut the copy.
   - **Partial**: the copy adds a condition, exception or detail.
     Merge it into the sentence that keeps the fact and cut the copy.
   - **Conflict**: two sources of one phase state it differently.
     Rewrite the losing side to the winning one, then treat the pair as identical.
   - **Inconsistency**: two phases state it differently, as the format one skill writes and another reads.
     Rewrite the losing side to the winning one.
   - **Misplaced**: a fact of the target belongs to another kind of source (see `Homes`).
     Move it to its home, merging it there.
2. Mark `external` a pair whose other source is outside the repository, and fix only the side of the repository.
3. Pick the winning side of a conflict or inconsistency from the code, the tests or the home of the fact, quoting it.
   Without such evidence, write no side and ask in Step 8.
4. Keep the fact in the source `Homes` gives it; without one between them, in the one that reaches more phases.
5. When the source that keeps a partial fact is outside the target and is not its home, merge nothing into it.
   Cut from the copy only the part that source states, and reword the rest.
6. List in `Kept`, as outside the target, every other fix that needs a file the target does not hold.
7. Mark a cut `on demand` when the source that keeps it misses a phase the copy reaches, naming those phases.
8. Cut from a skill or agent of a plugin only a copy its own files hold, since it runs in other repositories.

### Step 7: Apply the fixes to copies of the files in a temporary folder and verify them

- Copy each file to edit to a folder made with `mktemp -d`, keeping its path, and fix the copy.
- Cut a duplicate whole, with its sentence, list item or fenced code block, and leave no pointer to its source.
- Write each rewritten sentence in the language, terms, voice, layout and line wrapping of its file.
- Move a misplaced fact into the section of its home that holds its kind, never into a new section of its own.
- When a cut empties a section, remove its heading too, unless Step 6 found a link to it: then list the fact in `Kept`.
- Of the frontmatter, edit only the `description`, and list a finding in any other field in `Kept`.
- Never edit text a test or tool of Step 6 matches, and list its finding in `Kept`.
- Compare each copy with its original:
  ```sh
  diff -u "$original" "$copy"
  ```
- Restore every removed or reworded line that belongs to no finding.
- Undo every added line that belongs to no finding, and every fact a rewrite states that no source states.
- Reread each copy whole and fix every sentence a cut left pointing to text that is gone, such as "as above".

### Step 8: Show the findings and the diff, and wait for the confirmation

- Show `Conflicts`, `Inconsistencies` and `Misplaced` first, each with the side written and its evidence.
- Show the diff of each copy, marking each `on demand` cut with the phases it would leave.
- Name the files with uncommitted changes, which git cannot restore once overwritten.
- Name the files the workflow or a hook protects from edits: the user applies their diff by hand.
- Ask once which files to write and which side wins each open conflict.
- In the same question, ask whether to cut each `on demand` duplicate, keep it or replace it with a pointer.
- Write a pointer as one short sentence naming the file and section of the source that keeps the fact.
- Apply the answers to the copies, repeat the checks of Step 7, and show again only the diffs they change.
- Restore in the copies the fixes the user declines, and list them in `Kept`.
- Write only the files the user confirms, and nothing when the user declines.

### Step 9: Write the confirmed files over the originals, run the checks and report

- Write each confirmed copy over its original.
- When a hook denies an edit, as for a protected file, leave the original and give its diff in `Pending`.
- Run the tests or tools of Step 6 that read the files; restore the original of a file that breaks them.
- After editing the file of the workflow or the folder of rules, run the checks `AGENTS.md` or `CLAUDE.md` give.
- Report (see `Output`).

## References

### Sources

The folder of the AI is the one the project names in `AGENTS.md`, `CLAUDE.md` or the session, `ai/` by default.
The folder of rules is the one the project names the same way, `rules/` below the folder of the AI by default.

From the repository, tracked by git, edited only when in the target:

- `AGENTS.md` and `CLAUDE.md` of any folder.
- The skills and agents, with their supporting files.
- The Markdown files of the folder of the AI the workflow injects or a skill reads, such as context files and plans.
- The folder of rules and the file of the workflow of the agents, with the text and files each entry of it injects.
- The messages the workflow gives when it blocks an action: the agent reads them as instructions.
- The files a source imports with `@` and the files a step of a skill or agent tells to read, such as a design doc.

List the files with this command, with `ai/` replaced by the folder of the AI.
Set `$rules` to the folder of rules when it is outside the folder of the AI, and `$workflow` to the workflow file:

```sh
git ls-files -- '*AGENTS.md' '*CLAUDE.md' '*/skills/*' '*/agents/*' 'ai/*' ${rules:+"$rules"} \
  ${workflow:+"$workflow"} .claude/settings.json .codex/hooks.json
```

Then add the files of the last item above, found by reading the `@` imports and the steps of each source listed.
Of the Markdown files of the folder of the AI, such as plans, reports and tasks, keep only those a phase receives.

From outside the repository, read only:

- `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, and the skills and agents of the user and of the enabled plugins.
- An enabled plugin is one `enabledPlugins` sets to `true` in the settings.
  The first file that sets it wins: `.claude/settings.local.json`, `.claude/settings.json`, `~/.claude/settings.json`.
- Of an enabled plugin, only the version at its `installPath` in `~/.claude/plugins/installed_plugins.json`.
- Skip an enabled plugin whose `name` a `.claude-plugin/plugin.json` of the repository holds: it is a copy.
- Of a skill or agent no phase runs, only its `description`, which is all the session receives of it.
- The context the hooks, the harness and the organization gave this session, quoted from this session.

### Phases

A phase is a moment of the flow and what the agent receives in it.
Find them in the file of the workflow of the agents and in the skills that launch agents.
Find what the hooks inject in `.claude/settings.json` and `.codex/hooks.json`.

- **Session**: the main session at its start.
  It receives the global and root `CLAUDE.md` with their imports, or `AGENTS.md` for Codex.
  It also receives the descriptions of every enabled skill and agent and what the hooks inject at the start.
  It receives the `AGENTS.md` or `CLAUDE.md` of a subfolder only `on demand`, when it reads a file of that folder.
- **Skill**: one per skill the user or the workflow runs.
  It receives the session, its `SKILL.md`, the files its steps read and what the workflow injects when it starts.
- **Agent**: one per agent a skill launches, in a clean context.
  It receives its agent file, the prompt the skill passes and the files its steps read.
  It also receives what the workflow injects when it starts.
  Count the root `CLAUDE.md` in it to find conflicts, never as the source that keeps a cut.
- **Event**: what the workflow injects or blocks on a commit, an edit, a command or the end of a turn.
  It belongs to every phase where that event fires.

A source that does not reach every phase a copy reaches keeps that copy only `on demand`.

### Homes

The source of each kind of fact; a fact elsewhere is misplaced:

- How to write the code: the folder of rules, which the skill `rule` writes.
- The workflow of the agents, its phases and gates: the file of the workflow, which the skill `workflow` writes.
  Never move a fact into these two: leave it, and name in `Misplaced` the skill that writes it.
- What applies to the whole project: the root `AGENTS.md`, or `CLAUDE.md` when there is no `AGENTS.md`.
- What applies to one folder: its `AGENTS.md` or `CLAUDE.md`.
- What applies to one phase only: the skill or agent of that phase, or a file the workflow injects into it.
