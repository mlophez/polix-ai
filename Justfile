# Pass recipe arguments as $1, $2... so `just test -run X` reaches go test
set positional-arguments

# Vet and test the module
test *args:
    go vet ./... && go test ./... "$@"

# Build polix into build/polix (a static binary for this system)
build:
    go build -o build/polix ./cmd/polix

# Install polix with go install, so the hooks find it on the PATH
install:
    go install ./cmd/polix

# Install polix and check it against this repository: config, deterministic rules and machine
check: install
    polix check
    polix rules check
    polix doctor --offline

# Build the assets of a release into dist/ (what the release workflow publishes)
dist version:
    scripts/dist.sh "$1"

# Tag a release and push the tag: the release workflow builds and publishes it
release version:
    #!/bin/sh
    set -eu
    v="v${1#v}"
    test -z "$(git status --porcelain)" || { echo "the working tree is not clean" >&2; exit 1; }
    just test
    git tag -a "$v" -m "polix $v"
    git push origin "$v"
