---
title: Approval replay security test
description: Reproduce restart recovery, single-use approval, argument binding, and evidence tamper checks.
---

# Approval replay security test

This test runs built AegisFlow binary against local mock MCP upstream. It uses real HTTP endpoints, stops process after approval, then starts it again from same SQLite database.

![Approval replay E2E terminal run](../assets/approval-security-e2e.gif)

## Run it

Prerequisites: Go 1.26.10 or later, Node.js, `curl`, and `jq`.

```bash
make e2e-approval-security
```

Test uses temporary directory and random local port range. No external service or account is called.

## Checks

| Check | Expected result |
|---|---|
| Review request | Exact action enters pending queue |
| Process restart | Pending and approved state reloads from SQLite |
| Exact retry | Approved action reaches upstream once |
| Approval replay | Used approval does not reach upstream |
| Changed title | Changed arguments require new approval |
| Evidence reload | Four signed records verify after restart |
| Approval edit | Gateway rejects altered approval during startup |
| Evidence edit | Gateway rejects altered evidence during startup |

Harness also counts upstream calls. Replay and changed-argument requests pass only when upstream count remains one.

## Scope

Test covers routed MCP `tools/call` requests. It does not cover calls made outside configured gateway. SQLite backend supports one running AegisFlow process, not shared state across replicas.

Hash and signature checks detect record edits, reordered records, and missing records before chain tail. Detecting complete tail rollback requires checkpoint copied to separate append-only system.

Source: [`scripts/e2e_approval_security.sh`](https://github.com/saivedant169/AegisFlow/blob/main/scripts/e2e_approval_security.sh). GIF recorder: [`scripts/record-approval-security.sh`](https://github.com/saivedant169/AegisFlow/blob/main/scripts/record-approval-security.sh).
