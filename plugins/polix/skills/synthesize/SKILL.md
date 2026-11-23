---
name: synthesize
description: >
  Condenses a Markdown file in place, cutting filler, repetition and what its reader does not need, keeping its format,
  its readability and every instruction and fact.
  It runs only when the user asks for it.
  It never changes what the file says nor adds to it.
user-invocable: true
disable-model-invocation: true
argument-hint: "[path of a Markdown file, e.g. AGENTS.md, ai/plans/hooks.md; empty asks for it]"
effort: high
model: opus
---

# Synthesizing a Markdown file

Rewrite a Markdown file shorter, with the same information, format and meaning, and as easy to read.
Its reader is usually an agent: keep what changes what it does, cut the rest.

## Requirements

- [markdown.md](markdown.md) next to this file.
  Without it, stop and tell the user.

## Input

- **File**: the path of a Markdown file, from the arguments or the conversation.
  Without it, ask for it.

## Output

- **File**: its path, and its lines and words before and after.
- **Cut**: each kind of text removed, one line each with a short example.
- **Kept**: what looked removable but stayed because it carries information, or "none".
- **Deviations**: each place the original breaks [markdown.md](markdown.md), with its line and the fix, or "none".
- **Pending**: the checks that failed or were skipped, or "none".

## Limits

- Only edit the file of `Input`, and never stage nor commit it.
- Never drop an instruction, fact, condition, exception, decision, number, name, path, command nor link (see `Keep`).
- Never add information, advice nor opinion the original does not hold, nor change what a sentence means.
- Keep the format of the file (see `Keep`): condense the text inside each block, never its structure nor its layout.
- Keep the text readable (see `Readability`): a shorter sentence that is harder to follow is no gain.
- Write as [markdown.md](markdown.md) says, but never change the layout of the original for it: note it in `Deviations`.
- Keep the language and terminology of the file: never translate nor rename a term.
- When unsure whether a sentence carries information, keep it and list it in `Kept`.

## How to operate

Copy this checklist and track your progress:

```text
Synthesize progress:
- [ ] Step 1: Resolve the file (see `Input`) and read it whole.
- [ ] Step 2: Make sure the original can be restored (see Step 2).
- [ ] Step 3: Find what fixes its text: its reader, the links to it and the tools that parse it (see Step 3).
- [ ] Step 4: List every piece of information of the file, section by section (see `Keep`).
- [ ] Step 5: Rewrite each section, cutting what `Cuts` lists and keeping what `Keep` lists.
- [ ] Step 6: Write the result over the file.
- [ ] Step 7: Verify that nothing was lost (see Step 7).
- [ ] Step 8: Report (see `Output`).
```

### Step 2: Make sure the original can be restored

- Copy the file to a temporary file made with `mktemp`, to compare against in Step 7.
- If git does not track the file or it has uncommitted changes, ask the user before overwriting it.

### Step 3: Find what fixes its text

- Name its reader: an agent (a skill, `AGENTS.md`, `CLAUDE.md`, a plan, a rule, a prompt) or a person (a README, docs).
- Search the project with `rg` for links to its headings and for tests or tools that read it.
- Treat as fixed text what they need: frontmatter keys, required sections, exact sentences a test matches.

### Step 5: Rewrite each section, cutting what `Cuts` lists and keeping what `Keep` lists

- Read [markdown.md](markdown.md) before the first section.
- Work one section at a time, in order, for a file over 300 lines.
- When nothing of `Cuts` applies, leave the file untouched and say so.

### Step 7: Verify that nothing was lost

1. Compare the headings, links and inline code of both files:
   ```sh
   pattern='^#{1,6} .*|\]\([^)]*\)|`[^`]+`'
   diff <(rg -o "$pattern" "$original" | sort -u) <(rg -o "$pattern" "$file" | sort -u)
   ```
2. Compare the frontmatter and the fenced code blocks of both files: they must be identical.
3. Find each piece of information of Step 4 in the new file, and restore every one that is missing.
4. Reread the new file whole against `Readability` and the layout of `Keep`, and fix every line that breaks them.
5. Check the new file against [markdown.md](markdown.md), and with `markdownlint` when the project configures it.
   Fix what you wrote, and list in `Deviations` what the original already had.
6. For a file over 300 lines, launch a `general-purpose` subagent with both paths and this prompt, then repeat 3 and 4:
   ```text
   Compare the original Markdown file with its condensed version, reading both whole.
   List every instruction, fact, condition, exception, number, name, path, command or link the original holds and
   the condensed version lacks or states with another meaning, quoting the original line.
   List also every line of the condensed version harder to read than the original: telegraphic, ambiguous, with an
   abbreviation the original does not use, or with a layout that differs from the original.
   Answer "none" when nothing is lost nor harder to read; never list a cut of pure repetition or filler.
   ```
7. Run the tests or tools of Step 3 that read the file.

## References

### Cuts

- Preambles that announce the content, and closing summaries.
- Motivation, history and context the reader never acts on.
- Repetition: keep the most precise statement and drop the others, or point to it ("see `Section`").
- Filler and hedging: "it is important to note that", "basically", "simply", "in order to", "as mentioned above".
- Emphasis words and capitals that only add weight (`IMPORTANT`, `CRITICAL`), keeping the instruction they mark.
  Keep a normative level, as `MUST` and `SHOULD` of RFC 2119.
- Explanations of what the reader already knows: what git, Markdown or a well-known tool does.
- Several examples of one thing: keep the one that shows the most.
- Wordy phrasing: passive voice, nominalisations and long clauses, rewritten shorter with the same meaning.
- Emojis and decoration that carry no meaning.

### Keep

- The frontmatter and the fenced code blocks, byte for byte.
- The headings: their text, level and order.
- Links with their text and target, and images with their alt text.
- Numbers, versions, dates, units, names, paths, commands, identifiers, error messages and inline code, verbatim.
- Every instruction and constraint, with its conditions and exceptions ("only when", "unless", "except").
- A reason when, without it, the instruction would be misapplied.
- Quotes, legal or normative text, and text another file or a test matches, verbatim.
- The kind of each block: prose stays prose, a list keeps its order and nesting, a table its columns and rows.
- The layout: list markers, numbering, emphasis style, indentation, blank lines and the alignment of tables.
- The wrapping of lines: one sentence per line, or the same width as the original.

### Readability

- Write whole sentences with their articles and verbs: never a telegraphic style ("Run tests, check output").
- Use only the abbreviations and symbols the original uses: never `w/`, `->` nor `&` in place of words.
- Keep the connectors that carry the logic: "because", "unless", "so", "then".
- Keep one idea per sentence and the order of the original.
- Keep the tone and register of the original.
