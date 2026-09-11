#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
RUN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/aegisflow-approval-e2e.XXXXXX")"
BASE_PORT="${AEGISFLOW_E2E_BASE_PORT:-$((20000 + ($$ % 10000)))}"
GATEWAY_PORT="$BASE_PORT"
ADMIN_PORT="$((BASE_PORT + 1))"
MCP_PORT="$((BASE_PORT + 2))"
UPSTREAM_PORT="$((BASE_PORT + 3))"
API_KEY="approval-e2e-key"
EVIDENCE_KEY="approval-e2e-signing-key"
STATE_DB="$RUN_DIR/state.db"
CONFIG="$RUN_DIR/config.yaml"
BIN="$RUN_DIR/aegisflow"
AEGIS_PID=""
MOCK_PID=""
DEMO="${AEGISFLOW_DEMO:-0}"

if [[ "$DEMO" == "1" || -t 1 ]]; then
  BOLD='\033[1m'
  GREEN='\033[38;5;78m'
  YELLOW='\033[38;5;221m'
  RED='\033[38;5;203m'
  BLUE='\033[38;5;75m'
  RESET='\033[0m'
else
  BOLD=''
  GREEN=''
  YELLOW=''
  RED=''
  BLUE=''
  RESET=''
fi

cleanup() {
  if [[ -n "$AEGIS_PID" ]] && kill -0 "$AEGIS_PID" 2>/dev/null; then
    kill -TERM "$AEGIS_PID" 2>/dev/null || true
    wait "$AEGIS_PID" 2>/dev/null || true
  fi
  if [[ -n "$MOCK_PID" ]] && kill -0 "$MOCK_PID" 2>/dev/null; then
    kill -TERM "$MOCK_PID" 2>/dev/null || true
    wait "$MOCK_PID" 2>/dev/null || true
  fi
  rm -rf "$RUN_DIR"
}
trap cleanup EXIT INT TERM

fail() {
  printf '%bFAIL%b %s\n' "$RED" "$RESET" "$1" >&2
  if [[ -f "$RUN_DIR/aegisflow.log" ]]; then
    tail -30 "$RUN_DIR/aegisflow.log" >&2
  fi
  exit 1
}

pause_demo() {
  if [[ "$DEMO" == "1" ]]; then
    sleep 1
  fi
}

step() {
  printf '\n%b%s%b\n' "$BOLD" "$1" "$RESET"
}

pass() {
  printf '  %bPASS%b  %s\n' "$GREEN" "$RESET" "$1"
  pause_demo
}

port_is_open() {
  (echo >"/dev/tcp/127.0.0.1/$1") >/dev/null 2>&1
}

for port in "$GATEWAY_PORT" "$ADMIN_PORT" "$MCP_PORT" "$UPSTREAM_PORT"; do
  if port_is_open "$port"; then
    fail "port $port is already in use"
  fi
done

cat >"$CONFIG" <<YAML
server:
  host: "127.0.0.1"
  port: $GATEWAY_PORT
  admin_port: $ADMIN_PORT
  graceful_shutdown: 2s

providers:
  - name: "mock"
    type: "mock"
    enabled: true
    default: true

routes:
  - match:
      model: "*"
    providers: ["mock"]
    strategy: "priority"

tenants:
  - id: "approval-e2e"
    name: "Approval E2E"
    api_keys:
      - key: "$API_KEY"
        role: "admin"

tool_policies:
  enabled: true
  default_decision: "block"
  rules:
    - { protocol: "mcp", tool: "github.create_pull_request", decision: "review" }
    - { protocol: "mcp", tool: "github.delete_repo", decision: "block" }

state:
  enabled: true
  sqlite_path: "$STATE_DB"

mcp_gateway:
  enabled: true
  host: "127.0.0.1"
  port: $MCP_PORT
  require_auth: true
  upstreams:
    - name: "repo-mock"
      url: "http://127.0.0.1:$UPSTREAM_PORT"
      tools: ["github.*"]
YAML

wait_for_url() {
  local url="$1"
  for _ in $(seq 1 100); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.1
  done
  return 1
}

start_gateway() {
  AEGISFLOW_EVIDENCE_KEY="$EVIDENCE_KEY" \
    "$BIN" --config "$CONFIG" >"$RUN_DIR/aegisflow.log" 2>&1 &
  AEGIS_PID=$!
  wait_for_url "http://127.0.0.1:$GATEWAY_PORT/health" || fail "gateway did not become healthy"
}

stop_gateway() {
  kill -TERM "$AEGIS_PID"
  wait "$AEGIS_PID"
  AEGIS_PID=""
}

expect_startup_rejection() {
  local database="$1"
  local expected_log="$2"
  local log_file="$3"
  local state_name="$4"
  local pid
  local status

  AEGISFLOW_EVIDENCE_KEY="$EVIDENCE_KEY" AEGISFLOW_STATE_DB="$database" \
    "$BIN" --config "$CONFIG" >"$log_file" 2>&1 &
  pid=$!
  for _ in $(seq 1 50); do
    if ! kill -0 "$pid" 2>/dev/null; then
      status=0
      wait "$pid" || status=$?
      [[ "$status" -ne 0 ]] || fail "gateway exited cleanly with $state_name"
      grep -q "$expected_log" "$log_file" || fail "$state_name failed for an unexpected reason"
      return
    fi
    sleep 0.1
  done

  kill -TERM "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  fail "gateway accepted $state_name"
}

post_tool() {
  local id="$1"
  local arguments="$2"
  jq -nc \
    --argjson id "$id" \
    --argjson arguments "$arguments" \
    '{jsonrpc:"2.0",id:$id,method:"tools/call",params:{name:"github.create_pull_request",arguments:$arguments}}' |
    curl -fsS -X POST "http://127.0.0.1:$MCP_PORT/mcp" \
      -H "Content-Type: application/json" \
      -H "X-API-Key: $API_KEY" \
      --data-binary @-
}

expect_error_code() {
  local response="$1"
  local expected="$2"
  local actual
  actual="$(jq -r '.error.code // empty' <<<"$response")"
  [[ "$actual" == "$expected" ]] || fail "error code $actual, expected $expected"
}

if [[ "$DEMO" == "1" ]]; then
  printf '\033[2J\033[H'
fi
printf '%bAegisFlow approval replay test%b\n' "$BOLD" "$RESET"
printf 'Real process restart, SQLite state, signed evidence\n'

step "1. Build and start isolated test stack"
(cd "$ROOT" && go build -o "$BIN" ./cmd/aegisflow)
PORT="$UPSTREAM_PORT" node "$ROOT/scripts/mock-mcp-server.js" >"$RUN_DIR/mock.log" 2>&1 &
MOCK_PID=$!
wait_for_url "http://127.0.0.1:$UPSTREAM_PORT" || fail "mock upstream did not become healthy"
start_gateway
pass "gateway and upstream healthy"

approved_args='{"repo":"acme/widgets","title":"fix state restore","head":"fix/state","base":"main"}'
changed_args='{"repo":"acme/widgets","title":"changed after approval","head":"fix/state","base":"main"}'

step "2. Hold exact action for review"
first_response="$(post_tool 1 "$approved_args")"
expect_error_code "$first_response" "-32002"
approval_id="$(jq -r '.error.data.approval_id // empty' <<<"$first_response")"
[[ -n "$approval_id" ]] || fail "pending approval was not stored"
curl -fsS -X POST "http://127.0.0.1:$ADMIN_PORT/admin/v1/approvals/$approval_id/approve" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"reviewer":"e2e-reviewer","comment":"arguments checked"}' >/dev/null
pass "approval bound to exact arguments"

step "3. Restart gateway before execution"
stop_gateway
start_gateway
pass "approval and evidence restored from SQLite"

step "4. Retry approved action"
approved_response="$(post_tool 2 "$approved_args")"
jq -e '.error == null and (.result.content[0].text | fromjson | .number == 77)' \
  <<<"$approved_response" >/dev/null || fail "approved retry did not reach upstream"
pass "exact retry executed once"

step "5. Try approval replay"
replay_response="$(post_tool 3 "$approved_args")"
expect_error_code "$replay_response" "-32002"
upstream_calls="$(grep -c 'tools/call github.create_pull_request' "$RUN_DIR/mock.log" || true)"
[[ "$upstream_calls" == "1" ]] || fail "upstream call count $upstream_calls, expected 1"
pass "used approval rejected on replay"

step "6. Change one approved argument"
changed_response="$(post_tool 4 "$changed_args")"
expect_error_code "$changed_response" "-32002"
upstream_calls="$(grep -c 'tools/call github.create_pull_request' "$RUN_DIR/mock.log" || true)"
[[ "$upstream_calls" == "1" ]] || fail "changed arguments reached upstream"
pass "argument change requires new approval"

step "7. Verify restored evidence and tamper response"
session_id="$(curl -fsS "http://127.0.0.1:$ADMIN_PORT/admin/v1/evidence/sessions" -H "X-API-Key: $API_KEY" | jq -r '.[0].session_id')"
verify_response="$(curl -fsS -X POST \
  "http://127.0.0.1:$ADMIN_PORT/admin/v1/evidence/sessions/$session_id/verify" \
  -H "X-API-Key: $API_KEY")"
jq -e '.valid == true and .total_records == 4' <<<"$verify_response" >/dev/null || \
  fail "restored evidence did not verify"
pending_count="$(curl -fsS "http://127.0.0.1:$ADMIN_PORT/admin/v1/approvals" \
  -H "X-API-Key: $API_KEY" | jq '.pending | length')"
[[ "$pending_count" == "2" ]] || fail "pending approval count $pending_count, expected 2"
stop_gateway
APPROVAL_TAMPER_DB="$RUN_DIR/approval-tamper.db"
EVIDENCE_TAMPER_DB="$RUN_DIR/evidence-tamper.db"
cp "$STATE_DB" "$APPROVAL_TAMPER_DB"
cp "$STATE_DB" "$EVIDENCE_TAMPER_DB"
(cd "$ROOT" && go run ./tests/helpers/tamper-state approval "$APPROVAL_TAMPER_DB")
expect_startup_rejection \
  "$APPROVAL_TAMPER_DB" \
  "restore approval state" \
  "$RUN_DIR/approval-tamper-start.log" \
  "altered approval"
(cd "$ROOT" && go run ./tests/helpers/tamper-state evidence "$EVIDENCE_TAMPER_DB")
expect_startup_rejection \
  "$EVIDENCE_TAMPER_DB" \
  "restore evidence state" \
  "$RUN_DIR/evidence-tamper-start.log" \
  "altered evidence"
pass "altered approval and evidence rejected at startup"

printf '\n%bAll 7 approval security checks passed.%b\n' "$BLUE" "$RESET"
