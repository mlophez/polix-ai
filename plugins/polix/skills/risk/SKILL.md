---
name: risk
description: >
  Calibrates risk.weights and the thresholds changed_lines, changed_files and risk_score of polix.yaml.
  It measures past commits with polix risk --commit, so each gate catches the share of changes the team will review.
  It runs only when the user or the workflow of polix asks for it.
effort: medium
model: inherit
user-invocable: true
disable-model-invocation: true
argument-hint: "[share of commits to gate and commits to sample, e.g. 15% of the last 300; empty for 15% of 200]"
---

# Calibrating the risk

Measure the past commits with `polix risk --commit` and tune `risk.weights` and the measures of `polix.yaml`.
Each gate then holds on the share of commits the user accepts to review.

## Requirements

- `polix` on the `PATH` and a `polix.yaml` that `polix config check` accepts.
  Without them, stop and point to the skill `doctor` of polix.
- A history of at least 30 commits without merges in the scope of a gate.
  With fewer, stop and tell the user the numbers would be noise.

## Input

- **Target**: the share of commits each gate should hold on, from the arguments; 15% by default.
- **Sample**: how many commits without merges of the base branch to measure; the last 200 by default.
- **Critical paths**: the folders the user knows are risky (security, money, state, migrations), when they say so.

## Output

- **Sample**: the commits measured, the branch and the commits of each gate scope.
- **Weights**: each glob added, changed or removed, with its old and new weight and its evidence, or "none".
- **Thresholds**: per gate, each measure with its old and new number and the share of the sample it catches.
- **Caught**: the share of the sample each gate catches before and after.
- **Pending**: gates to add with the skill `workflow`, paths the user should confirm as critical, or "none".

## Rules

- Change only `risk.weights` and the numbers of `changed_lines`, `changed_files` and `risk_score` of existing entries.
  A new gate, condition or verb belongs to the skill `workflow` of polix.
- When no entry uses a measure, calibrate `risk.weights` alone and give the thresholds in `Pending`, for `workflow`.
- Measure each gate with its own scope: one `--changed` per glob of the `changed` that scopes its measures.
- Leave out of the statistics the commits whose scope did not change (`changed_files 0`).
- Recompute the score of a candidate weight from the rows of the measure, without writing `polix.yaml`.
  The score is the sum over the files of weight times log2(1 + added + deleted), rounded half away from zero.
- Give weight 0 to generated, vendored and lock files, and below 1 to tests and docs.
  Give above 1 only to paths with evidence of risk: fix or revert commits that touch them, or the critical paths.
- Keep weights between 0 and 5 and at most 10 globs; put a specific glob before the generic one it overrides.
- Round a threshold to a clean number (5, 10, 25, 50, 100, 150, 200, 300...).
- Treat a mechanical commit (a rename, a format of the whole tree, a release) as an outlier.
  Lower the weight of its files instead of raising a threshold for it.
- Never write a temporary file inside the working tree; use a temporary folder of the system.
- Talk to the user in the language of the session.

## How to operate

Copy this checklist and track your progress:

```text
Risk progress:
- [ ] Step 1: Run `polix config check`, read `risk.weights` and each entry that uses a measure, with its scope.
- [ ] Step 2: Resolve the input (see `Input`) and the base branch (`base_branch` of polix.yaml, else main).
- [ ] Step 3: Measure the sample once per gate scope (see `Measuring the history`).
- [ ] Step 4: Rank the paths by share of score and by fix and revert commits, and propose the weights.
- [ ] Step 5: Recompute the scores of the sample with the proposed weights.
- [ ] Step 6: Per gate, compute p50, p75, p90 and p95 of each measure and pick thresholds that catch the target.
- [ ] Step 7: Show the current and proposed values with the share each catches, and confirm once before writing.
- [ ] Step 8: Write the values, run `polix config check` and `polix config fmt` until both report no problem.
- [ ] Step 9: Measure the sample again and report the share each gate catches now.
```

### Step 6: Per gate, compute p50, p75, p90 and p95 of each measure and pick thresholds that catch the target

A gate whose measures sit under an `any` holds when one of them reaches its threshold, so the shares add up.
Lead with `risk_score`, which follows the weights.
Set `changed_lines` and `changed_files` near p95, as a net for large changes of low weight.
Count the commits the whole `any` would catch and adjust until it is within 5 points of the target.

### Step 7: Show the current and proposed values with the share each catches, and confirm once before writing

Show the weights added, changed and removed, with the evidence of each.
Show per gate the current and proposed thresholds and the share of the sample each catches.
List up to 10 commits the proposal catches and the current one does not, and up to 10 the other way round.
Give their subject and measures.

## References

### Measuring the history

`polix risk --commit` prints the commit, the three totals and one row per file (added, deleted, weight, score, path).
It uses the weights of the current `polix.yaml`.
Measure the sample into one file per gate, with the `--changed` of its scope:

```sh
for c in $(git log --no-merges --format=%h -n 200 main); do
  polix risk --commit "$c" --changed '*.go' --changed go.mod
done > "$TMPDIR/risk-go.txt"
```

On Windows without a POSIX shell, write the same loop with `foreach` in PowerShell.
Find the evidence of risk by path with the files of the fix and revert commits of the sample:

```sh
git log --no-merges -n 200 -i -E --grep '\b(fix|revert|hotfix)' --name-only --format= main \
  | sort | uniq -c | sort -rn
```
