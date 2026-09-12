#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT
mkdir -p "$TMP_DIR/bin" "$TMP_DIR/fixtures" "$TMP_DIR/install"
for binary in aegisflow aegisctl; do
  printf '#!/bin/sh\necho "%s v1.2.3"\n' "$binary" > "$TMP_DIR/fixtures/$binary-linux-amd64"
done
(cd "$TMP_DIR/fixtures" && shasum -a 256 aegisflow-linux-amd64 aegisctl-linux-amd64 > SHA256SUMS)
cat > "$TMP_DIR/bin/uname" <<'STUB'
#!/bin/sh
case "$1" in -s) echo Linux ;; -m) echo x86_64 ;; *) exit 1 ;; esac
STUB
cat > "$TMP_DIR/bin/curl" <<'STUB'
#!/bin/sh
set -eu
destination=""
url=""
while [ "$#" -gt 0 ]; do
 case "$1" in -o) destination="$2"; shift 2 ;; -*) shift ;; *) url="$1"; shift ;; esac
done
printf '%s\n' "$url" >> "$FIXTURE_DIR/requests"
case "$url" in
 https://api.github.com/repos/saivedant169/AegisFlow/releases/latest)
  printf '{"tag_name":"v1.2.3"}\n' > "$destination" ;;
 https://github.com/saivedant169/AegisFlow/releases/download/v1.2.3/SHA256SUMS)
  cp "$FIXTURE_DIR/SHA256SUMS" "$destination"
  case "${INSTALL_CASE:-}" in
   checksum) printf '%064d  aegisflow-linux-amd64\n' 0 > "$destination" ;;
   duplicate) cat "$FIXTURE_DIR/SHA256SUMS" >> "$destination" ;;
   missing) printf '%064d  absent\n' 0 > "$destination" ;;
  esac ;;
 https://github.com/saivedant169/AegisFlow/releases/download/v1.2.3/aegisflow-linux-amd64|https://github.com/saivedant169/AegisFlow/releases/download/v1.2.3/aegisctl-linux-amd64)
  [ "${INSTALL_CASE:-}" != download ] || exit 1
  cp "$FIXTURE_DIR/${url##*/}" "$destination" ;;
 *) exit 1 ;;
esac
STUB
chmod +x "$TMP_DIR/bin/uname" "$TMP_DIR/bin/curl"
run_install() {
 env PATH="$TMP_DIR/bin:$PATH" FIXTURE_DIR="$TMP_DIR/fixtures" AEGISFLOW_BIN_DIR="$TMP_DIR/install" AEGISFLOW_BINARY=aegisflow AEGISFLOW_VERSION=latest "$@" sh "$ROOT_DIR/scripts/install.sh"
}
run_install AEGISFLOW_VERSION=latest AEGISFLOW_BINARY=aegisflow > "$TMP_DIR/success.log"
test "$("$TMP_DIR/install/aegisflow" --version)" = 'aegisflow v1.2.3'
grep -q 'checksum verified' "$TMP_DIR/success.log"
test "$(grep -c 'releases/latest' "$TMP_DIR/fixtures/requests")" -eq 1
! grep -q 'latest/download' "$TMP_DIR/fixtures/requests"
run_install AEGISFLOW_VERSION=v1.2.3 AEGISFLOW_BINARY=aegisctl > "$TMP_DIR/cli.log"
test "$("$TMP_DIR/install/aegisctl" version)" = 'aegisctl v1.2.3'
for scenario in checksum duplicate missing download; do
 if run_install AEGISFLOW_VERSION=v1.2.3 AEGISFLOW_BINARY=aegisflow INSTALL_CASE="$scenario" > "$TMP_DIR/failure.log" 2>&1; then
  echo "installer accepted $scenario failure" >&2; exit 1
 fi
 test "$("$TMP_DIR/install/aegisflow" --version)" = 'aegisflow v1.2.3'
done
for tag in v01.2.3 'v1.2.3;id' $'v1.2.3\nv1.2.4'; do
 if run_install AEGISFLOW_VERSION="$tag" > "$TMP_DIR/failure.log" 2>&1; then echo 'invalid tag accepted' >&2; exit 1; fi
done
printf '#!/bin/sh\necho "aegisflow v1.2.2"\n' > "$TMP_DIR/fixtures/aegisflow-linux-amd64"
(cd "$TMP_DIR/fixtures" && shasum -a 256 aegisflow-linux-amd64 > SHA256SUMS)
if run_install AEGISFLOW_VERSION=v1.2.3 > "$TMP_DIR/failure.log" 2>&1; then echo 'wrong binary version accepted' >&2; exit 1; fi
grep -q 'binary version does not match' "$TMP_DIR/failure.log"
test "$("$TMP_DIR/install/aegisflow" --version)" = 'aegisflow v1.2.3'
echo 'installer verification tests passed'
