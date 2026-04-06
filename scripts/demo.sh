#!/bin/bash
# AegisFlow Demo - Agent Execution Governance
# This script demonstrates how AegisFlow controls agent actions

set -e

ADMIN_URL="${AEGISFLOW_ADMIN_URL:-http://localhost:8081}"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color
BOLD='\033[1m'

echo ""
echo -e "${BOLD}========================================${NC}"
echo -e "${BOLD}  AegisFlow Demo - Agent Governance${NC}"
echo -e "${BOLD}========================================${NC}"
echo ""

pause() {
    echo ""
    echo -e "${BLUE}Press Enter to continue...${NC}"
    read -r
}

echo -e "${BOLD}1. Agent reads GitHub repos (ALLOWED)${NC}"
echo -e "   Tool: github.list_repos | Protocol: git | Capability: read"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"git","tool":"github.list_repos","target":"aegisflow/aegisflow","capability":"read"}' | jq .
pause

echo -e "${BOLD}2. Agent tries to delete a repo (BLOCKED)${NC}"
echo -e "   Tool: github.delete_repo | Protocol: git | Capability: delete"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"git","tool":"github.delete_repo","target":"aegisflow/aegisflow","capability":"delete"}' | jq .
pause

echo -e "${BOLD}3. Agent creates a pull request (REVIEW REQUIRED)${NC}"
echo -e "   Tool: github.create_pull_request | Protocol: git | Capability: write"
echo ""
RESULT=$(curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"git","tool":"github.create_pull_request","target":"aegisflow/aegisflow","capability":"write"}')
echo "$RESULT" | jq .
ENVELOPE_ID=$(echo "$RESULT" | jq -r '.envelope_id')
pause

echo -e "${BOLD}4. Human reviews and approves the PR creation${NC}"
echo -e "   Checking pending approvals..."
echo ""
curl -s "$ADMIN_URL/admin/v1/approvals" | jq .
echo ""
echo -e "   Approving $ENVELOPE_ID..."
curl -s -X POST "$ADMIN_URL/admin/v1/approvals/$ENVELOPE_ID/approve" \
  -H "Content-Type: application/json" \
  -d '{"reviewer":"demo-admin","comment":"Looks good, approved"}' | jq .
pause

echo -e "${BOLD}5. Agent runs a safe shell command (ALLOWED)${NC}"
echo -e "   Tool: shell.pytest | Protocol: shell | Capability: execute"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"shell","tool":"shell.pytest","target":"/workspace","capability":"execute"}' | jq .
pause

echo -e "${BOLD}6. Agent tries rm -rf / (BLOCKED)${NC}"
echo -e "   Tool: shell.rm | Protocol: shell | Capability: delete"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"shell","tool":"shell.rm","target":"/","capability":"delete"}' | jq .
pause

echo -e "${BOLD}7. Agent runs SELECT query (ALLOWED)${NC}"
echo -e "   Tool: sql.select | Protocol: sql | Capability: read"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"sql","tool":"sql.select","target":"production_db","capability":"read"}' | jq .
pause

echo -e "${BOLD}8. Agent tries DROP TABLE (BLOCKED)${NC}"
echo -e "   Tool: sql.drop_table | Protocol: sql | Capability: delete"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"sql","tool":"sql.drop_table","target":"production_db","capability":"delete"}' | jq .
pause

echo -e "${BOLD}9. Verify audit chain integrity${NC}"
echo ""
curl -s -X POST "$ADMIN_URL/admin/v1/audit/verify" -H "X-API-Key: demo-key-001" | jq .
echo ""
echo -e "   ${BOLD}Approval history:${NC}"
curl -s "$ADMIN_URL/admin/v1/approvals/history" | jq '.history | length | "   \(.) actions reviewed"'
pause

echo ""
echo -e "${BOLD}========================================${NC}"
echo -e "${GREEN}${BOLD}  Demo complete!${NC}"
echo -e "${BOLD}========================================${NC}"
echo ""
echo "Summary:"
echo "  - Read operations: ALLOWED"
echo "  - Destructive operations: BLOCKED"
echo "  - Write operations: REVIEW REQUIRED -> APPROVED"
echo "  - Evidence chain: VERIFIED"
echo ""
