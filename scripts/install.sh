#!/bin/sh
# Download and verify one release binary without a Go toolchain.
# AEGISFLOW_VERSION: stable release tag, or latest (default).
# AEGISFLOW_BINARY: aegisflow (default) or aegisctl.
# AEGISFLOW_BIN_DIR: destination directory.
set -eu

REPO="saivedant169/AegisFlow"
BIN="${AEGISFLOW_BINARY:-aegisflow}"
VERSION="${AEGISFLOW_VERSION:-latest}"
err() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }
valid_tag() { printf '%s\n' "$1" | LC_ALL=C grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' && [ "$(printf '%s' "$1" | wc -l | tr -d ' ')" = 0 ]; }
case "$BIN" in aegisflow|aegisctl) ;; *) err "AEGISFLOW_BINARY must be aegisflow or aegisctl" ;; esac
if [ "$VERSION" != latest ]; then valid_tag "$VERSION" || err "invalid stable release tag"; fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in linux|darwin) ;; *) err "unsupported OS" ;; esac
arch=$(uname -m)
case "$arch" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) err "unsupported architecture" ;; esac
asset="${BIN}-${os}-${arch}"

tmpdir=$(mktemp -d)
staged=""
cleanup() { rm -rf "$tmpdir"; if [ -n "$staged" ]; then rm -f "$staged"; fi; }
trap cleanup 0
trap 'exit 1' 1 2 15

download() {
  if command -v curl >/dev/null 2>&1; then
    curl -fSL "$1" -o "$2" || err "download failed"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1" || err "download failed"
  else
    err "need curl or wget"
  fi
}

# Resolve latest once; all artifacts then come from the same tag URL.
if [ "$VERSION" = latest ]; then
  download "https://api.github.com/repos/${REPO}/releases/latest" "$tmpdir/latest.json"
  VERSION=$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmpdir/latest.json")
  valid_tag "$VERSION" || err "could not resolve a stable latest release tag"
fi
base="https://github.com/${REPO}/releases/download/${VERSION}"
info "downloading ${asset} (${VERSION})"
download "$base/$asset" "$tmpdir/$asset"
download "$base/SHA256SUMS" "$tmpdir/SHA256SUMS"

expected=$(awk -v name="$asset" '$2 == name { print $1 }' "$tmpdir/SHA256SUMS")
[ -n "$expected" ] || err "checksum missing for $asset"
printf '%s\n' "$expected" | LC_ALL=C grep -Eq '^[0-9a-f]{64}$' || err "invalid checksum entry"
[ "$(printf '%s\n' "$expected" | wc -l | tr -d ' ')" = 1 ] || err "duplicate checksum entry"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmpdir/$asset" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "$tmpdir/$asset" | awk '{print $1}')
else
  err "need sha256sum or shasum"
fi
[ "$actual" = "$expected" ] || err "checksum verification failed for $asset"
info "checksum verified"
chmod 755 "$tmpdir/$asset"
case "$BIN" in
  aegisflow) reported=$("$tmpdir/$asset" --version) || err "binary version check failed" ;;
  aegisctl) reported=$("$tmpdir/$asset" version) || err "binary version check failed" ;;
esac
[ "$reported" = "$BIN $VERSION" ] || err "binary version does not match release tag"

bindir="${AEGISFLOW_BIN_DIR:-/usr/local/bin}"
if [ -n "${AEGISFLOW_BIN_DIR:-}" ]; then
  mkdir -p "$bindir" || err "cannot create requested install directory"
elif [ ! -d "$bindir" ] || [ ! -w "$bindir" ]; then
  bindir="$HOME/.local/bin"
  mkdir -p "$bindir"
fi
[ ! -d "$bindir/$BIN" ] || err "install target is a directory"
# Stage on the destination filesystem so the final replacement is one rename.
staged=$(mktemp "$bindir/.${BIN}.XXXXXX")
cp "$tmpdir/$asset" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$bindir/$BIN"
staged=""
info "installed $bindir/$BIN ($VERSION)"
case ":$PATH:" in *":$bindir:"*) ;; *) info "add $bindir to PATH" ;; esac
