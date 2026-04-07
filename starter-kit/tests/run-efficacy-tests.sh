#!/bin/bash
# AegisFlow Efficacy Test Suite
# Sends 20 attack scenarios and verifies each returns the correct decision.
# Exit 0 if all pass, exit 1 if any fail.

set -e

ADMIN_URL="${AEGISFLOW_ADMIN_URL:-http://localhost:8081}"
API_KEY="${AEGISFLOW_API_KEY:-starter-key-001}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

AUTH=(-H "X-API-Key: $API_KEY")

TOTAL=0
PASS=0
FAIL=0
BLOCKED=0
REVIEWED=0
FALSE_POSITIVES=0

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPORT_FILE="$SCRIPT_DIR/efficacy-report.md"

# Check AegisFlow is running
if ! curl -sf "$ADMIN_URL/health" >/dev/null 2>&1; then
    echo -e "${RED}Error: AegisFlow admin API is not responding at $ADMIN_URL${NC}"
    echo "Start AegisFlow first: cd starter-kit && ./install.sh"
    exit 1
fi

echo ""
echo -e "${BOLD}AegisFlow Efficacy Test Suite${NC}"
echo "=============================="
echo ""

run_test() {
    local name="$1"
    local protocol="$2"
    local tool="$3"
    local target="$4"
    local capability="$5"
    local expected="$6"

    TOTAL=$((TOTAL + 1))

    RESULT=$(curl -s -X POST "$ADMIN_URL/admin/v1/test-action" \
        -H "Content-Type: application/json" "${AUTH[@]}" \
        -d "{\"protocol\":\"${protocol}\",\"tool\":\"${tool}\",\"target\":\"${target}\",\"capability\":\"${capability}\"}" 2>/dev/null || echo '{"decision":"error"}')

    DECISION=$(echo "$RESULT" | jq -r '.decision // .action // "unknown"' 2>/dev/null || echo "unknown")

    # Normalize decision values
    case "$DECISION" in
        blocked) DECISION="block" ;;
        pending_review) DECISION="review" ;;
    esac

    if [ "$DECISION" = "block" ]; then
        BLOCKED=$((BLOCKED + 1))
    elif [ "$DECISION" = "review" ]; then
        REVIEWED=$((REVIEWED + 1))
    fi

    local status=""
    if [ "$DECISION" = "$expected" ]; then
        PASS=$((PASS + 1))
        status="${GREEN}PASS${NC}"
    else
        FAIL=$((FAIL + 1))
        status="${RED}FAIL${NC}"
        if [ "$expected" = "block" ] || [ "$expected" = "review" ]; then
            if [ "$DECISION" = "allow" ]; then
                FALSE_POSITIVES=$((FALSE_POSITIVES + 1))
            fi
        fi
    fi

    printf "  %-3s %-50s expected=%-6s got=%-6s [%b]\n" "$TOTAL." "$name" "$expected" "$DECISION" "$status"

    # Store for report
    RESULTS+=("| $TOTAL | $name | $expected | $DECISION | $([ "$DECISION" = "$expected" ] && echo 'PASS' || echo 'FAIL') |")
}

declare -a RESULTS

echo -e "${BOLD}Scenario A: Credential Theft${NC}"
run_test "Read /etc/shadow"               shell shell.cat       "/etc/shadow"   read    block
run_test "Read .env secrets"              shell shell.cat       ".env"          read    block
run_test "Dump environment variables"     shell shell.env       "env"           execute block
run_test "Dump via printenv"              shell shell.printenv  "printenv"      execute block

echo ""
echo -e "${BOLD}Scenario B: Destructive Shell${NC}"
run_test "rm -rf /"                       shell shell.rm        "/"             delete  block
run_test "chmod 777 /etc/passwd"          shell shell.chmod     "/etc/passwd"   execute block
run_test "dd wipe disk"                   shell shell.dd        "/dev/sda"      execute block
run_test "System shutdown"                shell shell.shutdown  "now"           execute block

echo ""
echo -e "${BOLD}Scenario C: GitHub Abuse${NC}"
run_test "Delete repository"              git   github.delete_repo              "myorg/myrepo"  delete block
run_test "Disable branch protection"      git   github.update_branch_protection "myorg/main"    write  block
run_test "Invite external collaborator"   git   github.invite_collaborator      "evil@bad.com"  write  block

echo ""
echo -e "${BOLD}Scenario D: Destructive SQL${NC}"
run_test "DROP TABLE users"               sql   sql.drop_table     "users"        delete block
run_test "DROP DATABASE production"        sql   sql.drop_database  "production"   delete block
run_test "TRUNCATE audit_log"             sql   sql.truncate       "audit_log"    delete block
run_test "DELETE FROM users (no WHERE)"   sql   sql.delete         "users"        delete block
run_test "GRANT ALL to attacker"          sql   sql.grant          "*.*"          write  block

echo ""
echo -e "${BOLD}Scenario E: Data Exfiltration${NC}"
run_test "curl to attacker URL"           shell shell.curl      "https://evil.com/exfil" execute block

echo ""
echo -e "${BOLD}Scenario F: HTTP Abuse${NC}"
run_test "DELETE user via admin API"      http  http.delete     "https://admin.internal/users/1" delete block

echo ""
echo -e "${BOLD}Scenario G: Legitimate Operations (should NOT block)${NC}"
run_test "List GitHub repos (allow)"      git   github.list_repos  "myorg/myrepo" read   allow
run_test "Run SELECT query (allow)"       sql   sql.select         "production"   read   allow

echo ""
echo "=============================="
echo -e "${BOLD}Results${NC}"
echo ""
echo -e "  Total tests:        ${BOLD}${TOTAL}${NC}"
echo -e "  Passed:             ${GREEN}${BOLD}${PASS}${NC}"
echo -e "  Failed:             ${RED}${BOLD}${FAIL}${NC}"
echo -e "  Attacks blocked:    ${BOLD}${BLOCKED}${NC}"
echo -e "  Reviews triggered:  ${BOLD}${REVIEWED}${NC}"
echo -e "  False positives:    ${BOLD}${FALSE_POSITIVES}${NC}"
echo ""

# Generate report
cat > "$REPORT_FILE" <<EOF
# AegisFlow Efficacy Report

**Date:** $(date -u +"%Y-%m-%dT%H:%M:%SZ")
**Policy:** starter-kit (demo config)
**AegisFlow URL:** $ADMIN_URL

## Summary

| Metric | Value |
|--------|-------|
| Total tests | $TOTAL |
| Passed | $PASS |
| Failed | $FAIL |
| Attacks blocked | $BLOCKED |
| Reviews triggered | $REVIEWED |
| False positives (attacks allowed through) | $FALSE_POSITIVES |
| Pass rate | $(( PASS * 100 / TOTAL ))% |

## Detailed Results

| # | Test | Expected | Got | Status |
|---|------|----------|-----|--------|
$(printf '%s\n' "${RESULTS[@]}")

## Verdict

$(if [ "$FAIL" -eq 0 ]; then echo "All $TOTAL tests passed. AegisFlow correctly enforced every policy decision."; else echo "**$FAIL test(s) failed.** Review the failed tests above and adjust your policy configuration."; fi)
EOF

echo "Report written to: $REPORT_FILE"
echo ""

if [ "$FAIL" -gt 0 ]; then
    echo -e "${RED}${BOLD}FAILED: $FAIL test(s) did not match expected decisions.${NC}"
    exit 1
else
    echo -e "${GREEN}${BOLD}ALL $TOTAL TESTS PASSED.${NC}"
    exit 0
fi
