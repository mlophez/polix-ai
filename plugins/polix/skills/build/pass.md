---
plan: <path of the plan, relative to the root of the repository>
task: <T3, or final>
round: <0 for the first pass, else its number in its task: 1, 2...>
status: open
date: <today as YYYY-MM-DD>
findings: {}  # FND-NN: noted, for the reviewer's; open, fixed or skipped, for one the builder fixes
---

<!--
Template of the file of one pass of the build on a task or on the final verification.
Every <...> is replaced, every comment of this template is deleted, and a section with nothing to hold says "None.".
The frontmatter is what the build reads to resume; the body is the text of the reports, in their own words.
-->

# <T3, or Final>, <round 0 as "first pass", else "iteration <N>">: <title of the task, or of the plan>

## Task

<The status and evidence of the task the builder reports, one sentence per line, with its lines `Baseline:` and
`Departure:`; in a file of final, "None.".>

## Findings

### FND-01: <kind>, <where>

- **Source**: <verification | review | commit>
- **Kind**: <a verification area, a review severity and category, or the check that failed the commit>
- **Where**: <file:line, or the id of the plan it is about>
- **Repeats**: <the FND-NN of an earlier pass the report says it repeats, or none>

**Problem**

<What is wrong, with its instances and evidence, as the report says it.>

**Fix**

<What to change, as the report says it.>

**Evidence**

<What the builder did, fixed or skipped and why, with its lines `Departure:`; "None." while noted or open.>

## Acceptance

### AC-01

<The evidence of the verification for AC-01, as the verifier reports it; only in a file of final.>
