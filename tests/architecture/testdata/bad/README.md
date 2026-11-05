# testdata/bad

A tree that breaks every GOARCH rule with a deterministic check, so each test proves that its check detects the
violation. It is parsed, never compiled: the go.mod only names the module, so the tests know which imports are
of the module.

Conventions:

- Every violation carries a comment that names the rule it breaks, so a reader can tell why a line is there.
- The `want` strings of `tests/architecture/goarchNN_test.go` quote the messages of the checks; a few pin a line
  number, so append to a file rather than insert in it, and run `go test ./tests/architecture` after any change.
- GOARCH-8, 17 to 22 and 30 have no check: nothing here breaks them on purpose, and `goarch08_test.go` skips.
