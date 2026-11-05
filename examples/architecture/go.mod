// Separate module so the build, vet, tests and lint of the root module skip this example: it does not compile,
// it imports packages (pkg/git) that do not exist yet.
module github.com/mlophez/polix-ai/examples/architecture

go 1.25
