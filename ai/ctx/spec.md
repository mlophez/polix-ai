The method of this repository for the plans: every task that changes behaviour is built test first.

- A task's `Tests` are written before its `Files` change: each `New` test fails before the change, and it says what it
  checks, so the reason it fails first is clear.
- Test in two loops: the outer one checks the journeys and the criteria, the inner one the parts that build them.
- Outer loop: one `New` test per journey and per criterion the task covers, end to end through the public interface.
  It checks every observable effect of its `Then` and `But`.
  The effects are the output, the exit code or error returned and, when an input is rejected, the state unchanged.
- Inner loop: unit tests of the parts, `New` too, only where a part has logic the outer test cannot reach.
- Add a `Kept` test for each existing command, option or function that calls the code the task changes.
  Skip it when an existing test already covers that behaviour.
- A refactor kept by existing tests, or a change no test can observe such as docs, has no `New` tests.
  Its `Tests` list only the `Kept` tests that stay green, or "none" when none applies, and its summary says why.
