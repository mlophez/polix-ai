#!/bin/sh
# Builds the assets of a release into dist/: one raw binary per platform (polix_<os>_<arch>, .exe on Windows),
# install.sh, install.ps1 and checksums.txt with the SHA-256 of all of them, the names `polix update` and the
# install scripts expect.
#
#   scripts/dist.sh v0.2.0
set -eu

version=${1:?usage: scripts/dist.sh vX.Y.Z}
version=${version#v}
case $version in
  [0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "dist.sh: $version is not a version like 1.2.3" >&2; exit 1 ;;
esac

root=$(cd "$(dirname "$0")/.." && pwd)
out=$root/dist
rm -rf "$out"
mkdir -p "$out"

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64; do
  os=${target%/*}
  arch=${target#*/}
  ext=
  [ "$os" = windows ] && ext=.exe
  echo "building polix $version for $os/$arch"
  (cd "$root" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X github.com/mlophez/polix-ai/internal/cli.Version=$version" \
    -o "$out/polix_${os}_${arch}${ext}" ./cmd/polix)
done
cp "$root/install.sh" "$root/install.ps1" "$out/"

cd "$out"
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum polix_* install.sh install.ps1 > checksums.txt
else
  shasum -a 256 polix_* install.sh install.ps1 > checksums.txt
fi
cat checksums.txt
