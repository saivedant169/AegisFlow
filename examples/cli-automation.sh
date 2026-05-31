#!/usr/bin/env bash
#
# cli-automation.sh — drive AegisFlow from scripts and CI.
#
# Demonstrates three automation-friendly aegisctl workflows:
#   1. Test a policy WITHOUT executing the action or polluting the audit chain
#      (--dry-run).
#   2. Consume pending approvals as JSON and act on them programmatically
#      (pending --json + jq).
#   3. Gate a pipeline on cluster health with machine-readable status
#      (status --json + exit codes).
#
# Requires: aegisctl on PATH (or set AEGISCTL=./bin/aegisctl), jq.
#
# Nothing here mutates state except the optional approve/deny block, which is
# guarded behind APPROVE=1 so a plain run is safe.

set -euo pipefail

AEGISCTL="${AEGISCTL:-aegisctl}"

echo "==> 1. Dry-run a policy decision (no admin call, no audit, no queue)"
# Iterating on a policy pack? Check what WOULD happen before you commit it.
# A blocked tool prints decision=block but records nothing.
"$AEGISCTL" test-action --dry-run \
  --protocol mcp --tool github.delete_repo --target acme/widgets --capability delete

echo
echo "==> 2. Parse pending approvals in automation"
# `pending --json` emits the raw array. Pipe straight into jq.
PENDING_JSON="$("$AEGISCTL" pending --json)"
COUNT="$(printf '%s' "$PENDING_JSON" | jq 'length')"
echo "pending approvals: $COUNT"

if [ "$COUNT" -gt 0 ]; then
  # Pull the first item's id + tool for a decision.
  FIRST_ID="$(printf '%s' "$PENDING_JSON" | jq -r '.[0].id')"
  FIRST_TOOL="$(printf '%s' "$PENDING_JSON" | jq -r '.[0].envelope.tool')"
  echo "next item: id=$FIRST_ID tool=$FIRST_TOOL"

  # Safe by default: only act when APPROVE=1 is explicitly set.
  if [ "${APPROVE:-0}" = "1" ]; then
    case "$FIRST_TOOL" in
      github.create_pull_request|github.create_*)
        echo "auto-approving low-risk write: $FIRST_TOOL"
        "$AEGISCTL" approve "$FIRST_ID" "auto-approved by cli-automation.sh: low-risk write"
        ;;
      *)
        echo "denying unrecognized tool: $FIRST_TOOL"
        "$AEGISCTL" deny "$FIRST_ID" "auto-denied by cli-automation.sh: not in allowlist"
        ;;
    esac
  else
    echo "(set APPROVE=1 to act on this item; skipping)"
  fi
fi

echo
echo "==> 3. Gate a pipeline on health"
# status --json exits 0 healthy, 1 if gateway/admin down or chain invalid.
if "$AEGISCTL" status --json | jq -e '.healthy == true' >/dev/null; then
  echo "AegisFlow healthy — proceeding"
else
  echo "AegisFlow unhealthy — failing the pipeline" >&2
  exit 1
fi
