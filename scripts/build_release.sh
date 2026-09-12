#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
VERSION="${1:?usage: build_release.sh vMAJOR.MINOR.PATCH output-directory}"
OUTPUT="${2:?usage: build_release.sh vMAJOR.MINOR.PATCH output-directory}"
python3 "$ROOT/scripts/release_metadata.py" "$VERSION"
mkdir -p "$OUTPUT"
OUTPUT=$(cd "$OUTPUT" && pwd)
cd "$ROOT"
for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64; do
  os="${target%-*}"
  arch="${target#*-}"
  for binary in aegisflow aegisctl; do
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
      -ldflags="-s -w -X main.version=${VERSION}" \
      -o "$OUTPUT/${binary}-${target}" "./cmd/$binary"
  done
done
