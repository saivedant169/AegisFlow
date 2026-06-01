#!/bin/bash
# MCP stdio-to-HTTP bridge for AegisFlow.
#
# Claude Code (and other MCP clients) launch this as a stdio MCP server. It
# reads newline-delimited JSON-RPC requests on stdin, forwards each to
# AegisFlow's MCP gateway via HTTP POST, and writes the response to stdout.
#
# Failure handling: if the gateway is unreachable or returns nothing, the
# bridge replies with a proper JSON-RPC error (preserving the request id)
# instead of an empty line. An empty line would leave the client stuck on
# "connecting" with no diagnostic — the most common setup confusion. For
# notifications (requests with no id), no response is expected, so the bridge
# stays silent.

AEGISFLOW_MCP_URL="${AEGISFLOW_MCP_URL:-http://localhost:8082/mcp}"
CURL_MAX_TIME="${AEGISFLOW_MCP_TIMEOUT:-30}"

# extract_id pulls the JSON-RPC "id" token (string, number, or null) out of a
# request line. Prints the raw JSON token, or nothing if there is no id (which
# means the message is a notification and needs no response). Uses grep -oE for
# portability — BSD sed (macOS) does not support \| alternation.
extract_id() {
  printf '%s' "$1" \
    | grep -oE '"id"[[:space:]]*:[[:space:]]*("[^"]*"|[0-9]+|null)' \
    | head -1 \
    | sed -E 's/^"id"[[:space:]]*:[[:space:]]*//'
}

while IFS= read -r line; do
  # Ignore blank input lines.
  [ -z "$line" ] && continue

  response=$(curl -s --max-time "$CURL_MAX_TIME" -X POST "$AEGISFLOW_MCP_URL" \
    -H "Content-Type: application/json" \
    -d "$line" 2>/dev/null)

  if [ -n "$response" ]; then
    echo "$response"
    continue
  fi

  # Empty response: gateway down, timed out, or returned nothing.
  id=$(extract_id "$line")
  if [ -z "$id" ]; then
    # No id -> notification (e.g. notifications/initialized). No reply expected.
    continue
  fi

  # Reply with a clear, actionable JSON-RPC error so the client fails loudly.
  echo "{\"jsonrpc\":\"2.0\",\"id\":${id},\"error\":{\"code\":-32000,\"message\":\"AegisFlow MCP gateway unreachable at ${AEGISFLOW_MCP_URL}. Is AegisFlow running? Start it with 'make run' or './starter-kit/install-pr-writer.sh', then reconnect.\"}}"
done
