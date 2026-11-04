#!/bin/sh
# Installs polix from its GitHub Releases, checking the SHA-256 of the binary against checksums.txt.
#
#   curl -fsSL https://github.com/mlophez/polix-ai/releases/latest/download/install.sh | sh
#
# On macOS and Linux it installs the binary itself. On Windows (Git Bash, MSYS or Cygwin) it runs install.ps1
# of the same release, checked the same way, so a Windows machine gets one install wherever it starts from: in
# %LOCALAPPDATA%\Programs\polix and on the user PATH that every shell and Claude Code see.
#
# Environment:
#   POLIX_VERSION       version to install (default: the latest release)
#   POLIX_INSTALL_DIR   folder of the binary (default: ~/.local/bin; on Windows a Windows path, default
#                       %LOCALAPPDATA%\Programs\polix)
#   POLIX_RELEASES_URL  releases page (default: https://github.com/mlophez/polix-ai/releases)
set -eu

releases=${POLIX_RELEASES_URL:-https://github.com/mlophez/polix-ai/releases}
version=${POLIX_VERSION:-latest}
dir=${POLIX_INSTALL_DIR:-$HOME/.local/bin}

fail() { echo "install.sh: $*" >&2; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case $os in
  darwin|linux) ;;
  mingw*|msys*|cygwin*) os=windows ;;
  *) fail "unsupported system $os" ;;
esac
arch=$(uname -m)
case $arch in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $arch" ;;
esac
asset=polix_${os}_${arch}

if [ "$version" = latest ]; then
  base=$releases/latest/download
else
  base=$releases/download/v${version#v}
fi

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -O "$2" "$1"
  else
    fail "curl or wget is needed"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

# fetch downloads a file of the release into $tmp and checks it against checksums.txt.
fetch() {
  download "$base/$1" "$tmp/$1" || fail "cannot download $base/$1"
  want=$(awk -v a="$1" '$2 == a || $2 == "*" a { print $1 }' "$tmp/checksums.txt")
  [ -n "$want" ] || fail "checksums.txt has no entry for $1"
  got=$(sha256 "$tmp/$1")
  [ "$got" = "$want" ] || fail "checksum of $1 is $got, checksums.txt says $want"
}

download "$base/checksums.txt" "$tmp/checksums.txt" || fail "cannot download $base/checksums.txt"

if [ "$os" = windows ]; then
  fetch install.ps1
  script=$tmp/install.ps1
  command -v cygpath >/dev/null 2>&1 && script=$(cygpath -w "$script")
  # The environment (POLIX_VERSION, POLIX_INSTALL_DIR...) passes through to install.ps1.
  POLIX_RELEASES_URL=$releases powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$script"
  exit $?
fi

fetch "$asset"
mkdir -p "$dir"
chmod +x "$tmp/$asset"
mv "$tmp/$asset" "$dir/polix"
echo "installed polix $("$dir/polix" version) in $dir/polix"

case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "warning: $dir is not on the PATH; add it, or the hooks will not find polix" >&2 ;;
esac
