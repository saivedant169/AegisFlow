#!/bin/bash
# MCP stdio-to-HTTP bridge for AegisFlow
# Claude Code launches this as a stdio MCP server.
# It forwards JSON-RPC messages to AegisFlow's MCP gateway via HTTP POST.

AEGISFLOW_MCP_URL="${AEGISFLOW_MCP_URL:-http://localhost:8082/mcp}"

while IFS= read -r line; do
  # Forward the JSON-RPC request to AegisFlow and return the response
  response=$(curl -s -X POST "$AEGISFLOW_MCP_URL" \
    -H "Content-Type: application/json" \
    -d "$line" 2>/dev/null)
  echo "$response"
done
