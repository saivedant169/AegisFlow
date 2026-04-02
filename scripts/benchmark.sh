#!/usr/bin/env bash
set -euo pipefail

# Benchmark AegisFlow against the local mock provider.
# Usage:
#   scripts/benchmark.sh
# Optional env vars:
#   BENCH_REQUESTS=300
#   BENCH_CONCURRENCY=20
#   BENCH_P99_THRESHOLD_MS=250
#   BENCH_RESULTS_FILE=benchmark-results.txt

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REQUESTS="${BENCH_REQUESTS:-300}"
CONCURRENCY="${BENCH_CONCURRENCY:-20}"
P99_THRESHOLD_MS="${BENCH_P99_THRESHOLD_MS:-250}"
RESULTS_FILE="${BENCH_RESULTS_FILE:-$ROOT_DIR/benchmark-results.txt}"
TMP_DIR="$(mktemp -d)"
MOCK_LOG="$TMP_DIR/mock-provider.log"
GATEWAY_LOG="$TMP_DIR/aegisflow.log"
PAYLOAD_FILE="$TMP_DIR/payload.json"
CSV_FILE="$TMP_DIR/benchmark.csv"

cleanup() {
  if [[ -n "${GATEWAY_PID:-}" ]]; then
    kill "${GATEWAY_PID}" >/dev/null 2>&1 || true
    wait "${GATEWAY_PID}" 2>/dev/null || true
  fi
  if [[ -n "${MOCK_PID:-}" ]]; then
    kill "${MOCK_PID}" >/dev/null 2>&1 || true
    wait "${MOCK_PID}" 2>/dev/null || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

wait_for_http() {
  local url="$1"
  for _ in $(seq 1 60); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  echo "timed out waiting for $url" >&2
  exit 1
}

require_cmd curl
require_cmd go

if ! command -v hey >/dev/null 2>&1; then
  export GOBIN="$TMP_DIR/bin"
  mkdir -p "$GOBIN"
  go install github.com/rakyll/hey@latest
  export PATH="$GOBIN:$PATH"
fi

cat >"$PAYLOAD_FILE" <<'JSON'
{
  "model": "gpt-4o-mini",
  "messages": [
    {"role": "system", "content": "You are a concise assistant."},
    {"role": "user", "content": "Summarize why gateway benchmarking matters for regressions and performance tracking."}
  ],
  "temperature": 0.2,
  "max_tokens": 120
}
JSON

(
  cd "$ROOT_DIR"
  go run ./scripts/mock_provider.go -listen 127.0.0.1:18080 -latency 25ms >"$MOCK_LOG" 2>&1
) &
MOCK_PID=$!

(
  cd "$ROOT_DIR"
  go run ./cmd/aegisflow --config configs/benchmark.yaml >"$GATEWAY_LOG" 2>&1
) &
GATEWAY_PID=$!

wait_for_http "http://127.0.0.1:18080/health"
wait_for_http "http://127.0.0.1:18081/health"

START_NS="$(date +%s%N)"
hey -n "$REQUESTS" -c "$CONCURRENCY" \
  -o csv \
  -m POST \
  -H "Content-Type: application/json" \
  -H "X-API-Key: benchmark-key" \
  -D "$PAYLOAD_FILE" \
  http://127.0.0.1:18081/v1/chat/completions >"$CSV_FILE"
END_NS="$(date +%s%N)"

TOTAL_REQUESTS="$(tail -n +2 "$CSV_FILE" | wc -l | tr -d ' ')"
ELAPSED_SECONDS="$(awk -v start="$START_NS" -v finish="$END_NS" 'BEGIN { printf "%.6f", (finish - start) / 1000000000 }')"
RPS="$(awk -v total="$TOTAL_REQUESTS" -v elapsed="$ELAPSED_SECONDS" 'BEGIN { if (elapsed == 0) printf "0.0000"; else printf "%.4f", total / elapsed }')"
NON_2XX="$(awk -F, 'NR > 1 && $7 !~ /^2/ {count++} END {print count + 0}' "$CSV_FILE")"
ERROR_RATE="$(awk -v bad="$NON_2XX" -v total="$TOTAL_REQUESTS" 'BEGIN { if (total == 0) printf "0.00"; else printf "%.2f", (bad / total) * 100 }')"

percentile_seconds() {
  local percentile="$1"
  local index
  index="$(awk -v count="$TOTAL_REQUESTS" -v pct="$percentile" 'BEGIN { idx = int((pct / 100.0) * count + 0.999999); if (idx < 1) idx = 1; print idx }')"
  tail -n +2 "$CSV_FILE" | cut -d, -f1 | sort -n | sed -n "${index}p"
}

P50_SECONDS="$(percentile_seconds 50)"
P95_SECONDS="$(percentile_seconds 95)"
P99_SECONDS="$(percentile_seconds 99)"
P50_MS="$(awk -v secs="${P50_SECONDS:-0}" 'BEGIN { printf "%.2f", secs * 1000 }')"
P95_MS="$(awk -v secs="${P95_SECONDS:-0}" 'BEGIN { printf "%.2f", secs * 1000 }')"
P99_MS="$(awk -v secs="${P99_SECONDS:-0}" 'BEGIN { printf "%.2f", secs * 1000 }')"

{
  echo "Benchmark summary"
  echo "Requests/sec: ${RPS:-0}"
  echo "P50 latency (ms): $P50_MS"
  echo "P95 latency (ms): $P95_MS"
  echo "P99 latency (ms): $P99_MS"
  echo "Error rate (%): $ERROR_RATE"
  echo
  echo "Raw CSV"
  cat "$CSV_FILE"
} >"$RESULTS_FILE"

cat "$RESULTS_FILE"

if awk -v p99="$P99_MS" -v threshold="$P99_THRESHOLD_MS" 'BEGIN { exit !(p99 > threshold) }'; then
  echo "benchmark failed: p99 ${P99_MS}ms exceeded threshold ${P99_THRESHOLD_MS}ms" >&2
  exit 1
fi
