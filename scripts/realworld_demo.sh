#!/usr/bin/env bash
# Real-world MCP testing demo for AegisFlow
# Sends real MCP tool calls through AegisFlow to a mock upstream MCP server.
#
# Usage:
#   docker compose -f deployments/docker-compose.realworld.yaml up --build -d
#   ./scripts/realworld_demo.sh

set -euo pipefail

AEGISFLOW_URL="${AEGISFLOW_URL:-http://localhost:8080}"
ADMIN_URL="${ADMIN_URL:-http://localhost:8081}"
MCP_URL="${MCP_URL:-http://localhost:8082}"
API_KEY="${API_KEY:-realworld-key-001}"
BOLD='\033[1m'
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

divider() { echo -e "\n${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}\n"; }

echo -e "${BOLD}AegisFlow Real-World MCP Testing Demo${NC}"
echo "This demo sends MCP tool calls through AegisFlow to a mock GitHub MCP server."
echo "It exercises the full governance pipeline: allow, review, and block decisions."
divider

# ── Health checks ──
echo -e "${BOLD}[1/7] Health checks${NC}"
echo -n "  AegisFlow gateway... "
if curl -sf "${AEGISFLOW_URL}/health" > /dev/null 2>&1; then
  echo -e "${GREEN}OK${NC}"
else
  echo -e "${RED}FAILED${NC} (is AegisFlow running?)"
  echo "  Start with: docker compose -f deployments/docker-compose.realworld.yaml up --build -d"
  exit 1
fi
echo -n "  Mock MCP server...   "
if curl -sf "http://localhost:3000" > /dev/null 2>&1; then
  echo -e "${GREEN}OK${NC}"
else
  echo -e "${RED}FAILED${NC} (is mock-mcp running?)"
  exit 1
fi
divider

# ── Test 1: List repos (ALLOW) ──
echo -e "${BOLD}[2/7] github.list_repos — expected: ${GREEN}ALLOW${NC}"
echo "  Policy: github.list_* is allowed (read-only operation)"
echo ""
RESPONSE=$(curl -sf -X POST "${AEGISFLOW_URL}/v1/mcp" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${API_KEY}" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "tools/call",
    "params": {
      "name": "github.list_repos",
      "arguments": {"owner": "saivedant169"}
    }
  }' 2>&1) || RESPONSE=$(echo "$RESPONSE")
echo "  Response: $(echo "$RESPONSE" | jq -r '.result.content[0].text // .error.message // .' 2>/dev/null || echo "$RESPONSE")"
divider

# ── Test 2: List pull requests (ALLOW) ──
echo -e "${BOLD}[3/7] github.list_pull_requests — expected: ${GREEN}ALLOW${NC}"
echo "  Policy: github.list_* is allowed (read-only operation)"
echo ""
RESPONSE=$(curl -sf -X POST "${AEGISFLOW_URL}/v1/mcp" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${API_KEY}" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/call",
    "params": {
      "name": "github.list_pull_requests",
      "arguments": {"repo": "saivedant169/AegisFlow"}
    }
  }' 2>&1) || RESPONSE=$(echo "$RESPONSE")
echo "  Response: $(echo "$RESPONSE" | jq -r '.result.content[0].text // .error.message // .' 2>/dev/null || echo "$RESPONSE")"
divider

# ── Test 3: Create PR (REVIEW) ──
echo -e "${BOLD}[4/7] github.create_pull_request — expected: ${YELLOW}REVIEW${NC}"
echo "  Policy: github.create_pull_request requires human approval"
echo ""
RESPONSE=$(curl -sf -X POST "${AEGISFLOW_URL}/v1/mcp" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${API_KEY}" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "github.create_pull_request",
      "arguments": {"repo": "saivedant169/AegisFlow", "title": "Agent-created PR", "body": "Automated by coding agent"}
    }
  }' 2>&1) || RESPONSE=$(echo "$RESPONSE")
echo "  Response: $(echo "$RESPONSE" | jq '.' 2>/dev/null || echo "$RESPONSE")"

# Check for pending approval
echo ""
echo "  Checking approval queue..."
APPROVALS=$(curl -sf "${ADMIN_URL}/admin/v1/approvals" \
  -H "X-API-Key: ${API_KEY}" 2>&1) || APPROVALS="[]"
PENDING=$(echo "$APPROVALS" | jq 'length' 2>/dev/null || echo "0")
echo "  Pending approvals: ${PENDING}"

if [ "$PENDING" != "0" ] && [ "$PENDING" != "" ]; then
  APPROVAL_ID=$(echo "$APPROVALS" | jq -r '.[0].id // empty' 2>/dev/null)
  if [ -n "$APPROVAL_ID" ]; then
    echo ""
    echo -e "  ${YELLOW}Approving action ${APPROVAL_ID}...${NC}"
    APPROVE_RESP=$(curl -sf -X POST "${ADMIN_URL}/admin/v1/approvals/${APPROVAL_ID}/approve" \
      -H "X-API-Key: ${API_KEY}" \
      -H "Content-Type: application/json" \
      -d '{"approver": "demo-operator", "reason": "Approved during realworld demo"}' 2>&1) || APPROVE_RESP=""
    echo "  Approval result: $(echo "$APPROVE_RESP" | jq -r '.status // .' 2>/dev/null || echo "$APPROVE_RESP")"
  fi
fi
divider

# ── Test 4: Delete repo (BLOCK) ──
echo -e "${BOLD}[5/7] github.delete_repo — expected: ${RED}BLOCK${NC}"
echo "  Policy: github.delete_* is always blocked (destructive operation)"
echo ""
RESPONSE=$(curl -s -X POST "${AEGISFLOW_URL}/v1/mcp" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${API_KEY}" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "github.delete_repo",
      "arguments": {"repo": "saivedant169/my-app"}
    }
  }' 2>&1)
echo "  Response: $(echo "$RESPONSE" | jq '.' 2>/dev/null || echo "$RESPONSE")"
divider

# ── Test 5: Create branch (ALLOW) ──
echo -e "${BOLD}[6/7] github.create_branch — expected: ${GREEN}ALLOW${NC}"
echo "  Policy: github.create_branch is allowed (safe write operation)"
echo ""
RESPONSE=$(curl -sf -X POST "${AEGISFLOW_URL}/v1/mcp" \
  -H "Content-Type: application/json" \
  -H "X-API-Key: ${API_KEY}" \
  -d '{
    "jsonrpc": "2.0",
    "id": 5,
    "method": "tools/call",
    "params": {
      "name": "github.create_branch",
      "arguments": {"repo": "saivedant169/AegisFlow", "branch": "feature/agent-work"}
    }
  }' 2>&1) || RESPONSE=$(echo "$RESPONSE")
echo "  Response: $(echo "$RESPONSE" | jq -r '.result.content[0].text // .error.message // .' 2>/dev/null || echo "$RESPONSE")"
divider

# ── Test 6: Evidence chain verification ──
echo -e "${BOLD}[7/7] Evidence chain verification${NC}"
echo "  Verifying the tamper-evident evidence chain for this session..."
echo ""

# List evidence sessions
SESSIONS=$(curl -sf "${ADMIN_URL}/admin/v1/evidence/sessions" \
  -H "X-API-Key: ${API_KEY}" 2>&1) || SESSIONS="[]"
SESSION_COUNT=$(echo "$SESSIONS" | jq 'length' 2>/dev/null || echo "0")
echo "  Evidence sessions recorded: ${SESSION_COUNT}"

if [ "$SESSION_COUNT" != "0" ] && [ "$SESSION_COUNT" != "" ]; then
  SESSION_ID=$(echo "$SESSIONS" | jq -r '.[0].id // empty' 2>/dev/null)
  if [ -n "$SESSION_ID" ]; then
    echo "  Latest session: ${SESSION_ID}"

    # Verify chain integrity
    VERIFY=$(curl -sf -X POST "${ADMIN_URL}/admin/v1/evidence/sessions/${SESSION_ID}/verify" \
      -H "X-API-Key: ${API_KEY}" 2>&1) || VERIFY=""
    VALID=$(echo "$VERIFY" | jq -r '.valid // .verified // "unknown"' 2>/dev/null || echo "unknown")
    echo -e "  Chain integrity: ${GREEN}${VALID}${NC}"
  fi
fi

divider
echo -e "${BOLD}Demo complete.${NC}"
echo ""
echo "Summary of governance decisions:"
echo -e "  ${GREEN}ALLOW${NC}  — github.list_repos, github.list_pull_requests, github.create_branch"
echo -e "  ${YELLOW}REVIEW${NC} — github.create_pull_request (required approval)"
echo -e "  ${RED}BLOCK${NC}  — github.delete_repo (destructive, always blocked)"
echo ""
echo "All actions recorded in tamper-evident evidence chain."
echo "Inspect with: curl ${ADMIN_URL}/admin/v1/evidence/sessions"
