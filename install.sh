#!/bin/sh
# Aftcast installer for macOS and Linux.
#
#   curl -fsSL https://raw.githubusercontent.com/Hypership-Software/aftcast/main/install.sh | sh
#
# Downloads the release binary for this machine, verifies its checksum, and
# runs `aftcast init` - which installs the binary to ~/.aftcast/bin, adds it
# to PATH, starts the daemon, and wires the Claude Code hooks.
#
#   AFTCAST_VERSION   pin a release tag (default: latest), e.g.
#                     curl -fsSL .../install.sh | AFTCAST_VERSION=v0.1.0 sh
#   AFTCAST_BASE_URL  alternate release host for internal mirrors
#                     (default: https://github.com/Hypership-Software/aftcast/releases)

set -eu

REPO_URL="https://github.com/Hypership-Software/aftcast"

say()  { printf '%s\n' "$*"; }
fail() { printf 'install: %s\n' "$*" >&2; exit 1; }

detect_os() {
  case "$(uname -s)" in
    Darwin) echo darwin ;;
    Linux)  echo linux ;;
    *) fail "unsupported OS '$(uname -s)' - build from source instead: $REPO_URL#install-from-source" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64)  echo amd64 ;;
    arm64 | aarch64) echo arm64 ;;
    *) fail "unsupported architecture '$(uname -m)' - build from source instead: $REPO_URL#install-from-source" ;;
  esac
}

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    fail "need curl or wget to download Aftcast"
  fi
}

verify_checksum() {
  dir="$1" asset="$2"
  if command -v sha256sum >/dev/null 2>&1; then
    tool="sha256sum"
  elif command -v shasum >/dev/null 2>&1; then
    tool="shasum -a 256"
  else
    fail "need sha256sum or shasum to verify the download"
  fi
  (cd "$dir" && grep "[ *]$asset\$" checksums.txt | $tool -c - >/dev/null) ||
    fail "checksum mismatch for $asset - the download may be corrupted; try again"
}

main() {
  [ "$(id -u)" = "0" ] && [ -z "${AFTCAST_ALLOW_ROOT:-}" ] &&
    fail "aftcast is a per-user install - run without sudo (set AFTCAST_ALLOW_ROOT=1 to override)"

  os="$(detect_os)"
  arch="$(detect_arch)"
  asset="aftcast_${os}_${arch}.tar.gz"

  base="${AFTCAST_BASE_URL:-$REPO_URL/releases}"
  version="${AFTCAST_VERSION:-latest}"
  if [ "$version" = "latest" ]; then
    url="$base/latest/download"
  else
    url="$base/download/$version"
  fi

  tmp="$(mktemp -d 2>/dev/null || mktemp -d -t aftcast)"
  trap 'rm -rf "$tmp"' EXIT

  say "downloading Aftcast ($version, $os/$arch)..."
  fetch "$url/$asset" "$tmp/$asset"
  fetch "$url/checksums.txt" "$tmp/checksums.txt"
  verify_checksum "$tmp" "$asset"

  tar -xzf "$tmp/$asset" -C "$tmp"
  [ -x "$tmp/aftcast" ] || fail "release archive did not contain the aftcast binary"

  if [ ! -d "$HOME/.claude" ] && ! command -v claude >/dev/null 2>&1; then
    say "note: Claude Code was not detected - Aftcast will be ready once it is installed"
  fi

  "$tmp/aftcast" init

  say ""
  say "done. open a new terminal (so PATH picks up ~/.aftcast/bin), then:"
  say "  aftcast status    # daemon running, hooks wired"
  say "  aftcast doctor    # detailed checks"
  say "start a new Claude Code session and Aftcast observes it from there."
}

main
