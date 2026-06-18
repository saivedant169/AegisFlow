#!/usr/bin/env bash
# Real end-to-end HTTP load test: drives the actual server (HTTP + JSON codec +
# full governance pipeline) with a zero-latency mock provider, so the numbers
# reflect the gateway's own overhead — not provider or network latency.
#
# Requires: hey (https://github.com/rakyll/hey). Reproduce: ./scripts/loadtest_e2e.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

N="${N:-30000}"          # total requests
C="${C:-50}"             # concurrency
KEY="demo-key-001"
URL="http://localhost:8080/v1/chat/completions"

command -v hey >/dev/null || { echo "hey not installed: go install github.com/rakyll/hey@latest"; exit 1; }

go build -o bin/aegisflow ./cmd/aegisflow
AEGISFLOW_EVIDENCE_KEY="loadtest-key-0123456789abcdef" ./bin/aegisflow -config configs/loadtest.yaml >/tmp/aegis_loadtest.log 2>&1 &
PID=$!
trap 'kill -9 "$PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do curl -sf http://localhost:8080/health >/dev/null 2>&1 && break; sleep 0.1; done

echo "Running: hey -n $N -c $C against $URL (0-latency mock, governance on)"
hey -n "$N" -c "$C" -m POST \
  -H "X-API-Key: $KEY" -H "Content-Type: application/json" \
  -d '{"model":"mock","messages":[{"role":"user","content":"load test"}]}' \
  "$URL" | grep -E "Requests/sec|Total:|Slowest|Fastest|Average|50%|95%|99%|Status code|\[200\]"
