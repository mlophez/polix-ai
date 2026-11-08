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
    #!/bin/sh
    set -eu
    # Run the binary go install just wrote by its path, not the first polix of the PATH. go install writes to
    # GOBIN, else to the bin of the first entry of GOPATH (separated by ; on Windows, by : elsewhere).
    bin=$(go env GOBIN)
    if [ -z "$bin" ]; then
        sep=:
        [ "$(go env GOOS)" != windows ] || sep=';'
        gopath=$(go env GOPATH)
        bin=${gopath%%"$sep"*}/bin
    fi
    polix=$bin/polix
    [ -x "$polix" ] || polix=$polix.exe
    "$polix" config check
    "$polix" config fmt --check
    "$polix" rules check
    "$polix" doctor --offline

# Record a real Claude Code session with docs/assets/demo/demo.tape and turn it into docs/assets/demo.gif: idle
# frames dropped and the rest sped up. It needs vhs (with ttyd and ffmpeg) and Claude Code with the plugin polix.
demo speed="2":
    #!/bin/sh
    set -eu
    vhs docs/assets/demo/demo.tape
    ffmpeg -loglevel error -y -i docs/assets/demo.mp4 -filter_complex \
        "mpdecimate=hi=512:lo=256:frac=0.5,setpts=N/(12*$1*TB),fps=12,split[a][b];[a]palettegen=max_colors=96[p];[b][p]paletteuse=dither=none" \
        docs/assets/demo.gif

# Build the assets of a release into dist/ (what the release workflow publishes)
dist version:
    scripts/dist.sh "$1"

# Tag a release and push the tag: the release workflow builds and publishes it
release version:
    #!/bin/sh
    set -eu
    v="v${1#v}"
    test -z "$(git status --porcelain)" || { echo "the working tree is not clean" >&2; exit 1; }
    test "$(git rev-parse --abbrev-ref HEAD)" = main || { echo "releases are tagged on main" >&2; exit 1; }
    git fetch origin main
    test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" || { echo "HEAD is not origin/main: pull or push first" >&2; exit 1; }
    just test
    git tag -a "$v" -m "polix $v"
    git push origin "$v"
