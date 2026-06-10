#!/bin/sh
# AegisFlow installer — downloads a prebuilt binary from the GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/saivedant169/AegisFlow/main/scripts/install.sh | sh
#
# Options (env vars):
#   AEGISFLOW_VERSION   release tag to install (default: latest)
#   AEGISFLOW_BIN_DIR   install directory (default: /usr/local/bin, falls back to ~/.local/bin)
#
# No Go toolchain required.

set -eu

REPO="saivedant169/AegisFlow"
BIN="aegisflow"
VERSION="${AEGISFLOW_VERSION:-latest}"

err() { echo "error: $*" >&2; exit 1; }
info() { echo "==> $*"; }

# --- detect OS / arch ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux) os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS: $os (only linux and darwin have prebuilt binaries)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

asset="${BIN}-${os}-${arch}"

# --- resolve the download URL ---
if [ "$VERSION" = "latest" ]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

# --- pick an install dir we can write to ---
bindir="${AEGISFLOW_BIN_DIR:-/usr/local/bin}"
if [ ! -d "$bindir" ] || [ ! -w "$bindir" ]; then
  bindir="$HOME/.local/bin"
  mkdir -p "$bindir"
fi

tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT

info "downloading ${asset} (${VERSION})"
if command -v curl >/dev/null 2>&1; then
  curl -fSL "$url" -o "$tmp" || err "download failed: $url"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "$tmp" "$url" || err "download failed: $url"
else
  err "need curl or wget"
fi

chmod +x "$tmp"
mv "$tmp" "$bindir/$BIN"
trap - EXIT

info "installed $bindir/$BIN"
if ! command -v "$BIN" >/dev/null 2>&1; then
  echo "note: $bindir is not on your PATH — add it, e.g.:"
  echo "  export PATH=\"$bindir:\$PATH\""
fi
echo
echo "Next: grab a config and run it. The quickest governed demo (no API keys):"
echo "  git clone https://github.com/${REPO}.git && cd AegisFlow/starter-kit && ./install-pr-writer.sh"
echo "Or point a Claude client at the gateway: export ANTHROPIC_BASE_URL=http://localhost:8080"
