The method of this repository for the plans: every task that changes behaviour is built test first.

- A task's `tests` are written before its `files` change: each `new` test fails before the change, and its `checks`
  says what it checks, so the reason it fails first is clear.
- Test in two loops: the outer one checks the journeys and the criteria, the inner one the parts that build them.
- Outer loop: one `new` test per journey and per criterion the task covers, end to end through the public interface.
  It checks every observable effect of its `then` and `but`.
  The effects are the output, the exit code or error returned and, when an input is rejected, the state unchanged.
- Inner loop: unit tests of the parts, `new` too, only where a part has logic the outer test cannot reach.
- Add a `kept` test for each existing command, option or function that calls the code the task changes.
  Skip it when an existing test already covers that behaviour.
- A refactor kept by existing tests, or a change no test can observe such as docs, has no `new` tests.
  Its `tests` list only the `kept` tests that stay green, or `[]` when none applies, and its `summary` says why.
