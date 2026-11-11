# AnswerGithook

Answers one hook call of git: it evaluates the entries of the event of the section `githooks` of polix.yaml for the
commit being made, whoever makes it (a person, an IDE or an agent), and lets the commit go on or aborts it. It is
apart from the events of the AI (`on`, answered by `AnswerHook`): `on.pre_commit` is never evaluated here, and
`githooks` never there.

## Trigger

```
polix githook <pre-commit|commit-msg> [file] [--dir <dir>]
```

- `<pre-commit|commit-msg>`: the hook of git, mapped to the event of githooks it runs (`pre-commit` to
  `pre_commit`, `commit-msg` to `commit_msg`); any other name exits with 1 and the error.
- `file`: the file of the message, which git passes to `commit-msg`, relative to the working directory; `commit-msg`
  needs it and `pre-commit` takes none, else the command exits with 1 and the error.
- `--dir`: any folder of the repository; without it, the working directory, the root where git runs its hooks.

Git runs it from the hooks `polix install` declares in the local configuration of the repository
(`hook.polix-pre-commit` and `hook.polix-commit-msg`); hidden from the help, since people do not run it.

## Request

`Request`, filled by the command:

- `Event`, the event of githooks of the argument, `workflow.PreCommit` or `workflow.CommitMsg`.
- `File`, the file of the message of `commit-msg`; empty on `pre-commit`.
- `Dir`, from `--dir`.
- `Version`, the version of the running polix (`build.Current`), which polix.yaml may require.

## Logic

The use case takes the step shape: a run of an entry must have run before a later entry is evaluated, so the logic
returns the next run and the shell feeds its outcome back to the call (`githook.Githook.Ran`).

`Decide(g githook.Githook, wf workflow.Workflow, present bool, invalid error) Output`, with `g` the call of git (its
event, the branch, the staged files, on `commit_msg` the message as git wrote it and the comment string of git, and
the outcomes of the runs carried out so far, `githook.Githook.Outcomes`):

1. Without polix.yaml (`present` false) the commit goes on.
2. A workflow that cannot be used (`invalid`: a file that cannot be read, broken YAML, an unknown key, a newer polix
   it requires, or the problems of `workflow.Workflow.Valid`) aborts the commit, with each problem as a reason; a
   newer polix it requires says `run polix update`.
3. Else it walks the entries of `Workflow.Githooks` for the event, in file order. An entry applies when every
   condition of its `when` holds (an empty `when` holds) and, with an `unless`, not every condition of its `unless`
   does:
   - `branch`: a regular expression the current branch matches; a detached HEAD has the branch "".
   - `changed`: a glob, or a list of them, of which one matches a file staged for the commit, the index against
     HEAD (`internal/platform/glob`, as the `changed` of `on`).
   - `message`, only on `commit_msg`: a regular expression, or a list of them, of which one matches the first line
     of the message, never its body. The first line is taken as git commits the message: the lines that start
     with the comment string (`#`, or `core.commentChar` / `core.commentString`) and everything from the scissors
     line (`# ------------------------ >8 ------------------------`) on are removed, and so are the blank lines
     before it and the blanks at its end; it is "" when nothing is left. The merges and the other messages git
     writes are not told apart: an entry lets them through with `unless`.
   A pattern that does not compile once its placeholders are expanded does not hold.
4. A `block` that applies aborts the commit with its message.
5. A `run` that applies asks for its command (`Output.Run`), once per run carried out: when its outcome failed, it
   aborts the commit with its `message`, when it has one, and the line `<command> failed: <error>`; when it passed,
   the evaluation goes on with the next entry. Every run runs on every commit: githooks keep no log.
6. The first entry that aborts ends the evaluation; with none, the commit goes on.

The placeholders `{branch}`, `{message}` on `commit_msg` (the first line of the message) and the variables of `vars`
(the first group of their expression on the branch, or the whole match; one that does not match stays as written)
expand in the message of `block`, the command and the message of `run`, and the patterns of `changed` and
`message`; in the regular expressions of `message` their values are quoted (`regexp.QuoteMeta`) and matched
literally, so a branch such as `fix/c++` or `feat/a.b` neither breaks the pattern nor widens it. Its evaluation of the conditions and the placeholders is its own, apart from
that of `AnswerHook`.

`Output`: `Run`, the next command to run from the root, nil once the answer is ready; `Block`, whether the commit is
aborted; and `Reasons`, why, in order.

## Response

`Response`, from the output: `Reasons`, and `Code`, 1 when the output blocks, else 0. The command prints each line
of each reason to stderr prefixed with `polix: ` (`polix: No se commitea en main`), without the last line break of a
reason, and exits with `Code`: 0 lets the commit go on, 1 makes git abort it.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`, with the loader of polix.yaml and the writers
of the terminal of the commit:

- `Open`: opens the repository that contains the folder through `internal/platform/git`.
- `LoadWorkflow`: loads polix.yaml through the loader of `LoadWorkflow`, for the version of polix running: whether it
  exists, and the error of a file that cannot be used.
- `Branch`: the current branch (`git symbolic-ref`), also on a branch without commits, "" on a detached HEAD.
- `Staged`: the files staged for the commit, the index against HEAD, every file of the index on a repository without
  commits, deleted ones included and the files modified and not staged left out; git reads the index the hook is
  given (`GIT_INDEX_FILE`).
- `Message`: the content of the message file of `File`, read relative to the working directory, only on
  `commit_msg`.
- `CommentChar`: the comment string of the repository, only on `commit_msg`: `core.commentString`, else
  `core.commentChar`, else `#`; `auto` reads as `#`, since only git knows the character it picks.
- `Run`: runs a command from the root, never through a shell, without a time limit, with its stdout and stderr live
  to the terminal of the commit (`proc.Live`), and returns why it failed.

The adapter writes nothing to the repository: no log of polix under `<git-dir>/polix`.

## Errors

- An unknown event exits with 1 and the error, before the shell runs; the shell refuses it too
  (`githook.Githook.Valid`).
- `commit-msg` without the file of its message, or `pre-commit` with one, exits with 1 and the error.
- Staged files git cannot list, or a message file that cannot be read, are an error: the command prints it,
  prefixed `polix githook:`, and exits with 1.
- Outside a repository the commit goes on, exit 0 and nothing printed.

## Flow

1. `Command` maps the event of git to that of githooks, refuses an unknown one, and injects
   `Adapter{Workflow, Stdout, Stderr}` as the `Port`.
2. The shell validates the event (`githook.Githook.Valid`) and opens the repository; outside one it answers with
   nothing.
3. It loads polix.yaml, reads the branch and the staged files through the port and, with the file of a message,
   the message and the comment string, and builds the call (`githook.Githook`), followed by `Valid`.
4. It loops: `Decide` returns the next run, which the shell carries out through the port (`Run`) and feeds back
   with `githook.Githook.Ran`, until the output has no run.
5. It builds the response from the output; the command prints its reasons and exits with its code.
