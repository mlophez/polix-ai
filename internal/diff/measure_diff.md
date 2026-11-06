# MeasureDiff

Prints the measures the conditions `changed_lines`, `changed_files` and `risk_score` compare, over the changes not
committed yet or over the changes of one commit, then the share of each file in the risk score, so a person
calibrates their thresholds and the weights of `risk.weights`.

## Trigger

```
polix risk [--dir <dir>] [--changed <glob>]... [--commit <rev>]
```

- `--dir`: any folder of the repository; defaults to the current one.
- `--changed`: repeatable; measures only the files matching one of the globs, as the condition `changed` takes
  them. Without it, every changed file.
- `--commit`: any revision git resolves to a commit (a hash, `HEAD~3`, a tag); measures the changes that commit
  introduced instead of the uncommitted ones. Given empty, it is rejected before the shell runs.

## Request

`MeasureDiffRequest`, filled by the command from its flags:

- `Dir`, from `--dir`.
- `Changed`, the globs of `--changed`.
- `Commit`, the revision of `--commit`; empty for the uncommitted changes.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

`MeasureDiff(d Diff, weights []workflow.Weight, globs []string) MeasureDiffOutput` measures the diff. It takes what
the shell read through the port:

- `d`: the size of the change of each file, uncommitted or of the commit.
- `weights`: `Workflow.Weights`, the weights of `risk.weights` of the current polix.yaml, in file order.
- `globs`: the globs of `--changed`.

The rules it follows:

1. The diff is narrowed to the files matching one of the globs (`Diff.Narrow`); without globs it is whole.
2. `Lines` is the sum of the added and deleted lines (`Diff.Lines`), `Files` the number of changed files, deleted
   and binary included (`Diff.Count`), and `Risk` the risk score (`Diff.Risk`): the sum over the files of weight
   times log2(1 + added + deleted lines), rounded half away from zero.
3. The weight of a file is the factor of the first glob of the weights that matches it, else 1.
4. `Contributions` holds the share of each file (`Diff.Contributions`), the highest score first, then by path, and
   `Changes` the change of each of those files in the same order, for its added and deleted lines.

## Response

`MeasureDiffResponse`, from the output and what the port returned:

- `Commit`: the short hash and the subject of the commit measured; empty for the uncommitted changes.
- `Lines`, `Files` and `Risk`: the totals, `changed_lines`, `changed_files` and `risk_score`.
- `Rows`: one `MeasureDiffRow` per file, in the order of the contributions, with `Path`, `Added`, `Deleted`,
  `Binary`, `Factor` (its weight) and `Score` (its contribution before rounding).

The command prints what was measured (`uncommitted changes`, or `commit <hash> <subject>`), then the totals with
the names of the conditions, then, when a file changed, a header and one line per file: its added and deleted lines
(`-` for a binary file), its weight, its score and its path. With nothing to measure it prints zeros. It exits
with 0.

## Port

`MeasureDiffPort`, declared in `measure_diff_in.go` and implemented by `MeasureDiffAdapter` in
`measure_diff_out.go`:

- `LoadWorkflow`: reads polix.yaml with `workflow.Parse`; without the file, the default workflow, which has no
  weights. A file that cannot be read, does not parse, is not valid or requires a newer polix is an error.
- `Diff`: the size of the change of each file not committed yet (`git.Repo.Stats` against HEAD, untracked files
  read whole), as a validated `Diff`.
- `CommitDiff`: the size of the change of each file the commit introduced (`git.Repo.CommitStats`: against its
  first parent, or the empty tree for a root commit), as a validated `Diff`, with its short hash and subject
  (`git.Repo.Summary`).

## Errors

The use case fails, printing the error and exiting with 1, when:

- `--commit` is given empty.
- `--dir` is not inside a git repository.
- polix.yaml cannot be read, does not parse, is not valid or requires a newer polix.
- The revision of `--commit` is not a commit.
- git fails while measuring, or the diff it gives is not valid.

## Flow

1. `MeasureDiffCommand` rejects an empty `--commit`, opens the repository, turns the flags into a
   `MeasureDiffRequest` and injects `MeasureDiffAdapter{Repo: repo}` as the `MeasureDiffPort`.
2. The shell loads the workflow through the port, for its weights.
3. It reads the diff through the port: `CommitDiff` when the request has a revision, else `Diff`.
4. `MeasureDiff` measures it: `MeasureDiffOutput`.
5. The shell builds the response from the output, one row per contribution.
6. `MeasureDiffCommand` prints it.
