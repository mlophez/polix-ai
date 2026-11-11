# UpdateVersion

Replaces the running binary with a release of polix, checked against the checksums of the release, and refreshes
the plugin with the new binary; as the session start hook of the agents it keeps polix up to date without ever
failing.

## Trigger

```
polix update [--check] [--version <version>] [--hook]
```

- `--check`: only report: whether a newer release exists or, with `--version`, how that version compares to this
  binary.
- `--version`: install this version, older ones included, instead of the latest; a leading `v` is dropped.
- `--hook`: the session start hook: at most once an hour, quiet, never fails, then `polix doctor --hook`.
- `POLIX_NO_UPDATE`: when set, updates are disabled.
- `POLIX_RELEASES_URL` (the releases page, a mirror or the tests; default
  `https://github.com/mlophez/polix-ai/releases`) and `POLIX_CACHE_DIR` (the folder of the cache; default
  `<user cache dir>/polix`) are read by the adapter.

## Request

`Request`, filled by the command from its flags and the environment:

- `Check`, `Want` and `Hook`, from `--check`, `--version` (without its leading `v`) and `--hook`.
- `NoUpdate`, whether `POLIX_NO_UPDATE` is set.
- `Version`, the version of the running polix (`build.Current`).
- `Now`, the time of the call, against which the cache is read and written.

## Logic

`Decide(u version.Update, due bool, latest string, failed error) Output` decides what the update does and what it
tells, from the update asked (`version.Update`: the running version, the version wanted, the path of the binary and
the flags `--check`, `--hook` and `POLIX_NO_UPDATE`, which the shell builds and validates), whether the hook checks
now, the latest release and the error of asking it. In order:

1. Disabled: with `--hook` nothing; else the failure `updates are disabled by POLIX_NO_UPDATE`.
2. With `--hook` and not due: nothing to do, the plugin is refreshed.
3. Without `--version` (`towardLatest`): a failure to ask the latest release is the failure; a development build
   only reports the latest release with `--check`, and else fails asking for `--version`; a binary that is the
   latest or a later one is up to date (said only without `--hook`).
4. `--check` with `--version` (`compared`): the version is newer, the same or older than this binary (a
   downgrade), or this is a development build.
5. `--check` (`installing`): the binary can be updated to the latest release.
6. Else (`installing`) `Install` the target, telling `polix updated from <running> to <target> (<exe>)`, or with
   `--hook` `polix updated from <running> to <target>.`; with the latest release, `Remember` it as checked.

The versions are text, compared with `semver.Compare` and `semver.IsRelease` of `internal/platform/semver`: versions
like `1.2.3`, `v1.2.3` or `1.2.3-rc1`, where a missing number counts as 0, a pre-release is lower than its release
and a development build (`dev`) is no release.

`Refresh` is set when the plugin must be refreshed: without `--check`, unless the update fails. The policy on
failures is part of the output: `RecordFailure` (with `--hook`, a failure of asking the latest release or of the
install is recorded in the cache) and `RefreshAfterFailure` (with `--hook` and without `--check`, the plugin is
also refreshed after a failed install, except a replace that failed: `version.ErrReplace`). The shell asks them
through `Output.Record(err)` and `Output.RefreshAfter(err)`.

The domain decides the rest:

- `version.Cache.ShouldCheck`: the hook of a release build asks once every `version.CheckEvery` (an hour), at once
  when the cache knows a newer release or the workflow is rejected in a way a newer polix may fix
  (`version.Problems.NeedsNewer`: it requires a newer polix, names something unknown or has a later version of the
  schema), and never within `version.RetryAfter` (15 minutes) of a failed update; a development build never asks.
- `version.Cache.Saw`: a latest release seen counts as a check only when this binary is that release or a later
  one, so a check that leaves it behind (`--check`, a failed download) is repeated.
- `version.Cache.Fail`: the time of a failed update of the hook.

The cache compares its versions with the same semantics of `internal/platform/semver`.

## Response

`Response`, built from the output and what the port returned while carrying it out:

- `Hook`, whether it answers the session start hook.
- `Failure`, why the update failed, and `Message`, what it tells.
- `RefreshOut` and `RefreshErr`, what the refresh printed, and `Warning`, why it failed.
- `DoctorOut` and `DoctorErr`, what `doctor --hook` printed, and `DoctorFailure`, why it failed.

The command serializes it in that order: the failure to stderr as `polix update: <failure>`, the message to stdout,
the output of the refresh, its failure (`polix update: the plugin was not refreshed: <warning>`), the output of the
doctor and its failure (`polix doctor: <failure>`). With `--hook` the lines are plain and the exit code is always
0; else it exits with 1 when the update failed and with 0 otherwise.

## Port

`Port`, declared in `in.go` and implemented by `Adapter` in `out.go`:

- `Executable`: the real path of the running binary; it also removes `<exe>.old`, which a previous update on
  Windows moved aside.
- `LoadCache` and `SaveCache`: `update.json` in `POLIX_CACHE_DIR`, else `<user cache dir>/polix`; a cache that
  cannot be read is empty. `LoadCache` loads it through the field `Cache` of the adapter, the loader of
  `LoadVersion` (`internal/app/version/load`) that `cmd/polix` builds once and passes to `Command`; `SaveCache`
  encodes it with the unexported type `cacheJSON` of `out.go` (`checked_at`, `latest` and `failed_at` when an
  update failed), the shape `LoadVersion` decodes.
- `WorkflowRejected`: why this binary rejects the polix.yaml of the repository of the working folder, loaded
  through the field `Workflow` of the adapter, the loader of `LoadWorkflow` (`internal/app/workflow/load`) that
  `cmd/polix` builds once and passes to `Command`; nil outside a repository or without polix.yaml.
- `Latest`: the tag of the latest release, from the redirect of `<releases>/latest`, within 5 seconds for the hook
  and 2 minutes for a person.
- `Install`: checks that the folder of the binary can be written, downloads `polix_<os>_<arch>` (`.exe` on
  Windows) and `checksums.txt` of the release within 2 minutes, checks the binary and replaces the running one
  atomically (on Windows moving the old one aside first); a replace that fails is a `version.ErrReplace`.
- `Refresh`: runs the binary at the path, from the working folder: with `--hook` `__refresh --quiet`, else
  `install --skills --yes --quiet` and then `__refresh --hooks`.
- `Doctor`: runs `doctor --hook` with the binary at the path.

## Errors

Every error goes to the response; with `--hook` none of them makes the command fail:

- The path of the running binary cannot be found: nothing else runs.
- The latest release cannot be asked (no network, no release published), the folder of the binary cannot be
  written, the download or its checksum fails, or the replace fails: with `--hook` the failure is recorded in the
  cache, so the hook waits `version.RetryAfter` before trying again.
- The refresh or the doctor fail: told after the outcome of the update.

## Flow

1. `Command` turns the flags and `POLIX_NO_UPDATE` into a `Request` and injects
   `Adapter{Dir: ".", Hook: hook, Cache: cache, Workflow: loadWorkflow}`, with the loaders of the cache and of
   polix.yaml it was given, as the `Port`.
2. The shell reads the binary, builds the `version.Update` asked (one that is not valid is the failure of the
   response) and reads the cache and, with `--hook`, the workflow; when the update goes on without
   `--version` (with `--hook`, only when `version.Cache.ShouldCheck`), `askLatest` asks the latest release and
   saves what it saw (`version.Cache.Saw`), or with `--hook` the failure (`version.Cache.Fail`).
3. `Decide` decides what to do and tell: `Output`.
4. The shell carries out the output: `install` installs the target, recording a failure of the hook or, with the
   latest release, the check; the shell refreshes the plugin with the binary at the path, the new one when it was
   replaced; and with `--hook` `doctor` runs its doctor.
5. The shell builds the response.
6. `Command` serializes the response to the console and sets the exit code.
