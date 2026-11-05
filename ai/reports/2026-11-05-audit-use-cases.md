# Audit of the use cases

Date: 2026-11-05. Base: commit 15c1a46 (main), with the uncommitted change of polix.yaml (the `stop` block commented
out). Eleven auditors, one per use case identified by its `.md` in `internal/**`, with the shared files of each
package, the `pkg/` libraries it uses and the sections of `docs/polix.md` that describe it. Out of scope:
tests/architecture, ai/rules and docs/architecture.md.

## Incident

The auditor of AnswerHook ran a reproduction script inside the repository by mistake: a `cd` inside `$(...)` died in
a subshell and the following commands ran in the project. It overwrote `polix.yaml` and `.gitignore`, created `.env`,
`secret.txt` and `src/a.go`, and made a local commit (`5373789`, author `t <t@t>`) that included `build/polix`. It
undid it with `git reset --mixed 15c1a46` and restored the files. Verified afterwards: HEAD at 15c1a46 aligned with
origin/main, `git status` shows only ` M polix.yaml` with the same blob as before (`9174d2b..5d21c30`), `.gitignore`
equal to HEAD, no stray files. The orphan commit remains in the reflog with a 9.6 MB blob. To purge it:

```
git reflog expire --expire=now --all && git gc --prune=now
```

## Summary

- Areas: AnswerHook (internal/hook, internal/agent, pkg/shell), CheckRule (internal/rule, pkg/glob, pkg/git),
  CheckSetup (check_setup_*, setup.go, setup_problems.go, pkg/text), DiagnoseSetup (diagnose_setup_*,
  setup_finding.go, setup_hooks.go), InstallSetup (install_setup_*, setup_plugin.go, setup_settings.go, install.sh,
  install.ps1, tests/install), RefreshSetup (refresh_setup_*, plugins/plugins.go, pkg/file), UninstallSetup
  (uninstall_setup_*, setup_codex.go, setup_change.go), ShowVersion (show_version_*, version.go, version_parse.go,
  pkg/build, pkg/semver, pkg/cli, cmd/polix), UpdateVersion (update_version_*, version_cache.go, pkg/github,
  pkg/proc, scripts/dist.sh, release.yml), GraphWorkflow (graph_workflow_*, workflow*.go, examples/polix.yaml),
  LogWorkflow (log_workflow_*, internal/state, pkg/jsonl).
- Totals: 0 critical, 0 high, 25 medium, 56 low; 81 findings after merging 6 duplicates.
- Checks of the project: green.

The three that matter most:

1. The `protected` list stops applying when polix.yaml is invalid: any actor edits a protected, git-ignored `.env`
   when another entry has a mistake. The docs promise the opposite.
2. A check that is a Go test whose name does not match `Test{name}` always passes: `go test -run` exits 0 with no
   tests, so a deterministic rule goes unchecked in CI.
3. `file.Sync` deletes the marketplace folder when it is a symlink: install and refresh destroy a linked
   `~/.polix/marketplace` after writing into its target.

## Findings

### Medium, confirmed

- **AnswerHook, inconsistency with a security effect.** `internal/hook/answer_hook_logic.go:129-131` and
  `:195-199`. `protect()` only takes `Workflow.Protected` when the workflow is valid; with `Invalid`,
  `outsideChange()` lets ignored files through. Reproduced: an edit of `.env` by `builder` exits 0; with a valid
  workflow it blocks. Contradicts docs/polix.md:191-195 and answer_hook.md Logic rule 1. Direction: use
  `Protected` also when the error is a `workflow.Invalid`, since Parse returns the populated workflow.
- **AnswerHook, bug.** `internal/hook/hook_evaluation.go:42-45` and `answer_hook_logic.go:682-699`. `changed` and
  `files` are read once per call; only the hash, the contents and `contains` go by epoch. An entry with `changed`
  after a `run` that creates files decides on the stale list. Reproduced with two entries in `stop`. Contradicts
  answer_hook.md and docs/polix.md:421-422. Direction: index `Changed` and `Files` by epoch like `Hashes`.
- **AnswerHook, inconsistency.** `internal/agent/agent_answer.go:58-64`. When the answer blocks, `Warnings` are
  dropped, including the unknown-hash warning. A stop blocked by `unless: done` never says the fact will not be
  recorded, and the agent repeats until the loop guard. Direction: print `polix: <warning>` on stderr also when
  blocking, and align the Response section of the spec.
- **CheckRule, robustness.** `polix.yaml:16`, `internal/rule/check_rule_out.go:65-67`, docs/polix.md:277-278. The
  runner `go test -run "^Test{name}$"` exits 0 when no test matches. Reproduced: `✓ 1 checks, 0 failed` with
  `[no tests to run]`. Direction: detect `no tests to run` in the output, or document the limit and the guard of
  `tests/architecture/rules_checks_test.go`.
- **CheckRule, inconsistency.** docs/polix.md:267-268 and :306-307 vs `internal/workflow/workflow_runner.go:54`
  and `:91`. `rules.run` matches with `path.Match` on the base name, without `**`; every other glob uses
  `pkg/glob`. A runner `**/*_test.go` passes `polix check` and then runs the `_test.go` as a program. Direction:
  document the dialect or unify it, and have `polix check` warn on runner globs with `/`.
- **CheckRule, inconsistency.** `internal/rule/check_rule.md:74`, `check_rule_in.go:50`, `check_rule_out.go:51` say
  "the first base that exists"; `pkg/git/git.go:85-88`, docs/polix.md:46-48 and the test implement "the closest".
  Direction: fix the spec and the port comments.
- **CheckRule, bug.** `pkg/git/git.go:95-98` and `:113-114`. When the base exists but has no merge base (orphan,
  shallow clone), `ResolveBase` drops it as missing and the message sends the user to polix.yaml. Reproduced with
  an orphan branch. The `break` also skips the fallback to `origin/<name>`. Direction: tell "missing" from "no merge
  base" and suggest `fetch-depth: 0`.
- **GraphWorkflow, inconsistency.** `internal/workflow/graph_workflow_logic.go:91`. Conditions are written with
  `%v`: `not=map[branch:^main$]`, `changed=[*.go go.mod]`. `graph_workflow_logic_test.go:24` asserts it.
  Direction: lists joined by comma and `not` recursive.
- **GraphWorkflow, inconsistency.** `internal/workflow/workflow_parse.go:26`. `Protected []string` only accepts a
  list; `base` and the condition globs accept a scalar. `protected: "ai/rules/**"` fails with a raw yaml error
  that, not being `Invalid`, blocks fail-closed. Direction: declare it as `stringsYAML`.
- **GraphWorkflow, inconsistency.** `internal/workflow/workflow_parse.go:84-93` and `:261-305`. The
  `UnmarshalYAML` with `type plain X` make the errors name `workflow.plain` (`run: just test`, `rules: [go]`,
  `context: hola`). `decodeNames` and its test do not cover them. Direction: check `MappingNode` and return a
  message of its own with the line.
- **DiagnoseSetup, bug.** `internal/setup/diagnose_setup_logic.go:143-147`. An empty or blank `settings.json`
  fails with "unexpected end of JSON input"; `readHooks` and `decodeObject` treat it as no hooks. Direction: read
  the Claude settings with `readHooks`, as the Codex ones.
- **DiagnoseSetup, inconsistency.** `internal/setup/diagnose_setup_logic.go:161-169`, `:188`, `:241-252` vs
  `setup.go:129-140`. Install counts any polix command as a duplicate; doctor only `polix hook claude`. Two
  `polix update --hook` in parallel pass doctor. A command with an absolute path passes the events but fails
  "does not run polix". Direction: one shared predicate.
- **DiagnoseSetup, CheckSetup and LogWorkflow, inconsistency (merged).** docs/polix.md:60-67 says `doctor`,
  `install` and `uninstall` take `--dir` with default "the cwd of the hook input". None reads stdin; only
  `polix hook` does. And `check`, `graph`, `log` and `rules check` also take `--dir` through `cli.DirFlag`.
  `log_workflow.md:9` documents `--dir` without `-C`. Direction: drop the cwd sentence and list every command.
- **UninstallSetup, inconsistency.** `internal/setup/uninstall_setup_in.go:60-62` and `:88`. The help of
  `--agent` copies install's ("codex when it is installed") but uninstall removes both; the `Long` says it removes
  `~/.polix/marketplace/` when it only does so with both agents. Direction: help of its own.
- **UninstallSetup, bug.** `internal/setup/setup_codex.go:11-20` and `:40-75`. The TOML is parsed line by line:
  `enabled = true # on` reads false, `[plugins.'polix@polix']` is not recognized and subtables are orphaned.
  Reproduced with 9 cases. Direction: normalize quotes, cut at `#`, include the subtables.
- **LogWorkflow, bug.** `pkg/jsonl/jsonl.go:65`. The last line without `\n` of `limit+1` bytes is not skipped.
  Reproduced with a 4,194,305-byte line. Direction: compare against `limit` when the chunk does not end in `\n`.
- **LogWorkflow, inconsistency.** docs/polix.md:36 says `--json` prints "the raw lines"; `log_workflow_in.go:106`
  re-serializes with the known fields and drops those of a newer polix. Direction: fix the doc or return the
  original line.
- **ShowVersion, inconsistency.** docs/polix.md:138-139 and `pkg/build/build.go:10-11` say `just install` never
  yields a release; `build.go:26-34` takes the module tag and Go stamps it on a clean checkout. Reproduced:
  `polix version` prints `0.9.9`. Direction: document it or set `-X build.Version=dev` in `just build`.
- **RefreshSetup, bug.** `pkg/file/file.go:104-123`. `Sync` on a symlinked dir does not follow it, `Rel` gives
  `.`, marks it `stale` and `os.Remove`s the link after writing into the target. Reproduced. Affects `WritePlugin`
  of refresh and install. Direction: `EvalSymlinks` before the `WalkDir`, and a test.
- **CheckSetup, inconsistency.** `pkg/text/text.go:11`. `Mark = "… [recortado]"` in Spanish reaches the output of
  `rules check` and the hook answers. Breaks GO-2. Direction: an English marker.
- **InstallSetup, inconsistency.** `install_setup_in.go:119` and docs/polix.md:453 say Codex "when it is
  installed"; `install_setup_out.go:41-47` includes it when `~/.codex` or `<root>/.codex` exists, a folder install
  itself creates and uninstall does not remove. Direction: align help and doc with "used", or narrow `CodexUsed`.
- **UpdateVersion, bug.** `update_version_in.go:85`, `update_version_logic.go:79`, `pkg/github/github.go:71`.
  `--version` is not validated: `latest` compares as `0.0.0` ("would be a downgrade") and `../../x` ends in the
  URL. Direction: validate `Want` in the entry.

### Medium, likely

- **RefreshSetup, inconsistency.** `refresh_setup_out.go:62-71` and `refresh_setup_in.go:101-103`.
  `codex plugin marketplace upgrade` only runs when `WritePlugin` changed something; on a manual `polix update`,
  `install --user` already wrote the plugin and `__refresh --hooks` sees no change. Codex never gets the upgrade
  except through the hook. docs/polix.md does not mention the command. Direction: decide where the Codex upgrade
  lives and document it.
- **UninstallSetup, robustness.** `uninstall_setup_out.go:56-69`. Codex: the error of `marketplace remove` is
  discarded and it only runs when `CodexEnabled`. Claude without CLI: `return nil, nil` with no `todo` (install
  leaves one). Then `RemovePlugin` deletes the registered folder. Direction: treat a missing or failing CLI as
  install does.
- **UninstallSetup, bug.** `uninstall_setup_out.go:71-81`. If `marketplace remove` fails after `plugin
  uninstall`, the change already made does not reach `Changes`; if the plugin was gone but the marketplace was
  there, it is removed without a report. Direction: accumulate the changes before returning the error.

### Low, confirmed

- **AnswerHook and LogWorkflow, inconsistency (merged).** `answer_hook_logic.go:62` logs the builtin protection
  with outcome `deny`; docs/polix.md:413 lists only block, warn, audit, allow, loop guard. Direction: use `block`
  or document `deny` and the action `builtin`.
- **AnswerHook, inconsistency.** `answer_hook.md` Logic omits the `Contains` step of `hook_step.go:23-24`.
- **AnswerHook, tests.** `answer_hook_out_test.go:820-845` only ties the timeout of `examples/claude-settings.json`
  to `RunBudget`; `examples/codex-hooks.json` and `.claude/settings.json` may diverge silently.
- **CheckRule, robustness.** `check_rule_out.go:22-23`. On the 5-minute timeout the error is `signal: killed` with
  no mention of the timeout. Direction: wrap with "timed out after 5m".
- **CheckRule, inconsistency.** `pkg/git/git.go:51-59`. `Changed` includes uncommitted and untracked files; the
  docs say "what the branch changed" and that on main it checks nothing.
- **CheckRule and CheckSetup, security and robustness (merged).** `rule.go:27-41` only rejects absolute paths;
  `check_setup_out.go:38-40` takes `../x.sh` or a directory as existing. `polix check` says ok and `rules check`
  fails with "permission denied". Direction: reject `..` and anything that is not a regular file.
- **CheckSetup, inconsistency.** `setup_problems.go:34`. The missing check is only reported when the rule is
  valid; with a rule without description it appears after fixing that. Contradicts "every problem, not only the
  first".
- **CheckSetup, tests.** `check_setup_in_test.go:67,75`. `CLICOLOR_FORCE=1 go test -run TestCheckSetupCommand`
  fails on the color codes. Direction: clear the color variables in `home(t)`.
- **CheckSetup, inconsistency.** `pkg/text/text.go:13-25`. `Cut` returns the whole `Mark` with `limit < 15`,
  against its comment.
- **CheckSetup, bug.** `pkg/text/text.go:36-38`, `:46-47`. `Tail` and `LastLines` panic with `n < 0`.
- **CheckSetup, inconsistency.** `check_setup_in.go:58-59`. The `Long` only lists workflow problems, not those of
  ai/rules/.
- **CheckSetup, stale.** `setup.go:15-16`. The package comment says it is pure; the `*_out.go` adapters live in
  the package since 15c1a46.
- **CheckSetup, inconsistency.** `check_setup.md` Port names `rule.ParseAll`; the code uses `rule.ParseFilesAll`.
- **GraphWorkflow, inconsistency.** `graph_workflow_logic.go:57-74`. `context` and `allow` are drawn without
  detail: three different `context` entries come out identical.
- **GraphWorkflow, bug.** `graph_workflow_logic.go:57-78`. An entry without action yields
  `n0["if changed=*.go<br/>"]` or `n0[""]`.
- **GraphWorkflow, inconsistency.** `graph_workflow_logic.go:26-30`. Entries under an unknown event vanish from
  the graph with exit 0; the spec says it draws the workflow "as it is".
- **GraphWorkflow, inconsistency.** docs/polix.md:314 does not say `allow` is rejected in session_start,
  post_edit and post_command (`workflow_entry.go:186-189`).
- **GraphWorkflow, tests.** `workflow_test.go:133`. No test validates the polix.yaml of the repository and CI only
  runs `rules check`.
- **DiagnoseSetup, robustness.** `diagnose_setup_logic.go:282-284` and `setup.go:98-124`. With `Home == ""` the
  plugin check is silent and `CodexHooksFiles`, `CodexConfigFile` and `PluginManifest` read paths relative to the
  cwd.
- **DiagnoseSetup, inconsistency.** docs/polix.md:147-148 "skips both" is not what `RuleProblems` does: only the
  executable and interpreter checks are skipped.
- **DiagnoseSetup, inconsistency.** `diagnose_setup.md:55-56` omits the `exeErr == nil` condition, the plugin
  folder check and the one-minute limit of `claude plugin list`.
- **DiagnoseSetup, tests.** `diagnose_setup_logic_test.go` does not exercise `ExeErr`, `LatestErr`, an unreadable
  or invalid settings file, the `UserPromptSubmit` timeout, a disabled plugin nor a broken `hooks.json`;
  `diagnose_setup_in_test.go:96-98` indexes `calls[0]` without checking the length.
- **InstallSetup, robustness.** `setup_settings.go:155-162` and `:44-49`. A `settings.json` with a UTF-8 BOM or
  `"hooks": null` fails with "not a JSON object" after the plugin was written. Reproduced.
- **InstallSetup, inconsistency.** `install_setup_out.go:84-88` vs `:101-107`. A Codex CLI failure is "run by hand"
  and exit 0; a Claude one is exit 1. Neither spec nor docs say so.
- **InstallSetup, inconsistency.** `install_setup_out.go:111-114`. `claude plugin update` leaves no `Change`.
- **InstallSetup, inconsistency.** `install_setup_in.go:118,123` and `uninstall_setup_in.go:82` register `--dir`
  by hand; the rest use `cli.DirFlag`.
- **InstallSetup, tests.** `install_setup_in_test.go:48-67` never completes the shell with `User: true`;
  `tests/install` does not exercise `POLIX_VERSION`, `POLIX_NO_SETUP` nor the `wget` fallback.
- **UninstallSetup, inconsistency.** `uninstall_setup_out.go:75` removes the marketplace without `--scope user`
  while install adds it with it; the CLI says "Omit to remove it from every scope".
- **UninstallSetup, inconsistency.** docs/polix.md:471-473 and `uninstall_setup.md` omit that it deletes
  `~/.codex/config.toml` when it only held the table, `~/.polix` when empty, the Codex condition and the
  cancellation with exit 1.
- **UninstallSetup, robustness.** `uninstall_setup_in.go:104-107`. `Root` returns "" on any git error and the
  command says "nothing to remove"; install warns "not inside a git repository".
- **UninstallSetup, tests.** `TestUninstallSetupShell` only counts calls; no coverage of `--agent codex`, a missing
  CLI, a failing `marketplace remove`, nor `Changes`/`Synced`.
- **RefreshSetup, inconsistency.** `refresh_setup.md:36-37` omits that Codex requires `CodexEnabled`.
- **RefreshSetup, stale.** `refresh_setup_in.go:92`. Without `--hooks` it reads `.claude/settings.json` and
  `.codex/hooks.json` relative to the cwd, dead work the logic discards.
- **RefreshSetup, tests.** No test of the `Failure` branch, of port errors, of `UpdatePlugin` with a failing CLI,
  nor of `Sync` with a symlink or a partial failure.
- **LogWorkflow, bug.** `internal/state/state_line.go:41`. `null` and `{}` yield ghost entries without kind that
  `polix log` prints as blank rows.
- **LogWorkflow, tests.** No cases for a last line without `\n` over the limit, non-object lines, nor `print` of
  an `injected` entry.
- **ShowVersion and UpdateVersion, robustness (merged).** `scripts/dist.sh:12` uses a shell pattern that accepts
  `1.2.3.4` or `1.2.3beta`; `Justfile:43-50` pushes the tag before validating; `pkg/semver/semver.go:41-49`
  parses those values lossily. Direction: validate with the regex of `semver.valid` before `git tag`.
- **ShowVersion, dead.** `internal/version/version.go:37-57`. `Version.Compare` is used by nobody but its test;
  the use cases use `semver.Compare` with another semantics. Three regexes for the same format.
- **ShowVersion, dead.** `pkg/build/build.go:17,30`. The `pseudoVersion` filter is unreachable: `releasedModule`
  never matches a pseudo-version.
- **ShowVersion, inconsistency.** `show_version.md:3-4` names "the update of an older polix" as a consumer, which
  does not run `polix version`; the real ones are `install.sh:98`, `install.ps1:111` and `release.yml`.
- **ShowVersion, stale.** AGENTS.md Layout lists `internal/` as "agent, cli, engine, gitx, glob, rules, setup,
  shell, state, update, version, workflow"; the real packages are agent, hook, rule, setup, state, version,
  workflow.
- **UpdateVersion, inconsistency.** `update_version_out.go:67-80` writes `update.json` with `os.WriteFile`;
  `diagnose_setup_out.go:59-69` with the atomic `file.Write`. Two concurrent hooks may read a truncated JSON.
- **UpdateVersion, security.** `update_version_out.go:143-149` and `pkg/github/github.go:70-91`.
  `POLIX_RELEASES_URL` accepts `http` and redirects to any scheme; the checksum comes from the same origin. No
  impact with the default. Direction: require https except loopback.
- **UpdateVersion, inconsistency.** docs/polix.md:123-124 says "five seconds to reach GitHub" when the download
  has 2 minutes; `update_version.md:52` omits that with `--hook` a failed `Latest` or a dev build also refresh.
- **UpdateVersion, tests.** `github_test.go:116` and `update_version_out_test.go:549` tolerate an orphan temp file
  with `> 2`; no tests of the `fetch` limits nor of a non-version tag.

### Low, likely

- **RefreshSetup and UpdateVersion, robustness (merged).** `update_version_out.go:160-161` runs `__refresh` and
  `doctor` without timeout; `:108` uses 2 minutes of download also in hook mode; each CLI has 1 minute. The worst
  case exceeds the 180 s of the hook and Claude Code kills it, possibly between the two renames on Windows.
  Direction: bound `run` and the hook budget.
- **GraphWorkflow, robustness.** `graph_workflow_logic.go:79`. Only `"` is escaped; a label starting with a
  backtick opens a Mermaid markdown string. Render not verified.
- **DiagnoseSetup, dead.** `diagnose_setup_logic.go:305-307`. The folder check uses `readFromFolder`, a key that
  `claude plugin list --json` 2.1.318 does not print; it never fires.
- **CheckRule, tests.** `check_rule_out_test.go:35-42`. The git tests do not isolate `GIT_CONFIG_GLOBAL`; on this
  machine the global `pre-commit` runs on every test commit.
- **CheckSetup, robustness.** `setup.go:143-148`. `quote` does not escape `"` in paths shown to the user.
- **InstallSetup, inconsistency.** docs/polix.md:446-448 justifies `claude plugin update` with Claude Code copying
  to its cache; 2.1.318 loads in place from the marketplace.
- **InstallSetup, inconsistency with XPLAT-1.** `install_setup_in.go:185,212-213,226` print native paths; the
  rule asks for forward slashes.
- **UninstallSetup, bug.** `uninstall_setup_out.go:114`. `os.Remove` deletes the symlink instead of the target,
  unlike `file.Write`.
- **RefreshSetup, robustness.** `refresh_setup_out.go:42-50`. An unreadable file is treated as missing and
  `__refresh` stays silent.
- **RefreshSetup, bug.** `refresh_setup_in.go:117-119`. With `--quiet --hooks` the message says "plugin" even when
  only the hooks changed.
- **UpdateVersion, robustness.** `pkg/github/github.go:55-67`. `Latest` does not validate that the tag is a
  version; a 302 to a captive portal is reported as "no release published yet" and a non-version tag invalidates
  the cache on every session.
- **LogWorkflow, robustness.** `pkg/jsonl/jsonl.go:51`. `Append` ignores the error of `Close`.

## Checks

Run by the auditor of AnswerHook:

- `go vet ./... && go test ./...`: every package ok.
- `golangci-lint run ./...`: 0 issues.
- Not run: `just check` (its `go install` would replace the installed binary) and `claude plugin validate`.

Every auditor built the binary in the scratchpad and reproduced its findings in temporary repositories. InstallSetup
and UninstallSetup also verified the real contract of `claude plugin ... --json` (Claude Code 2.1.318) in an
isolated HOME.

## Coverage

Read whole: the eleven `.md`, all of `internal/**` with its tests, all of `pkg/**`, `cmd/polix/main.go`,
`plugins/plugins.go`, install.sh, install.ps1, tests/install, scripts/dist.sh, both CI workflows, Justfile, examples/,
ai/rules and the relevant sections of docs/polix.md and README. Only skimmed: the content of `plugins/polix/` (skills
and agents). Not audited: tests/architecture, docs/architecture.md and docs/use-cases.md beyond the recipes the tests
validate.

## Pending

- Codex is not installed on the machine: neither `codex plugin marketplace add/remove/upgrade` nor the format of
  `marketplace.json` were checked. It affects the real severity of the Codex findings in install, uninstall and
  refresh.
- Windows not reproduced: `install.ps1`, the BOM of PowerShell 5.1, `Runnable` and `RunCheck` only by reading and
  simulated tests.
- Mermaid rendering not verified (no `mmdc`).
- The default timeouts of Claude Code that doctor assumes (600 s, 30 s) were not checked against its documentation.
- The uncommitted change of polix.yaml (the `stop` block commented out) will leave its header and
  docs/polix.md:388-396 stale if committed as is.
- Decide whether to purge the orphan commit of the reflog, and which findings to fix.
