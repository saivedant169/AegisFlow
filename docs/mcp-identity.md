# MCP identity and approval boundaries

Development branch behavior. These changes have not been released.

`POST /mcp` accepts direct JSON-RPC. `GET /sse` creates an authenticated SSE connection; messages use its returned `/mcp/session/{id}` endpoint. Other paths cannot dispatch tool calls. `GET /health` and `HEAD /health` remain public; `POST /health` cannot execute tools.

Set `mcp_gateway.require_auth: true`. Send `X-API-Key` or `Authorization: Bearer <key>`. Each key has its configured tenant and role. The gateway derives a stable principal identifier from that tenant and key without storing the raw key in evidence. Key rotation creates a new principal and requires new approvals.

Direct requests can set `X-AegisFlow-Session-ID` to separate tasks. This label is scoped to the authenticated tenant and principal, so reusing another client's label grants no authority. Omitting it selects that principal's default HTTP session. Retries must use the same key and session label. Clients sharing one key and label share identity; assign separate keys to independent agents.

Each SSE connection has a distinct session scope. Only its authenticated principal can submit messages to it. Approval retries must use that same live connection. Reconnecting creates a new scope and requires fresh approval. HTTP and SSE approvals cannot cross transports.

Reviewed tool calls require authentication even when `require_auth` is false. Unauthenticated direct calls still follow configured allow/block policy; disabling authentication does not create isolated anonymous identities. SSE always requires authentication.

## Review and evidence access

Approval lists, history, individual approvals, evidence exports, verification, and reports require authentication and return only the caller's tenant data. Operators and admins can approve or deny within their own tenant. Viewer keys cannot review. The recorded reviewer comes from the authenticated principal; a JSON `reviewer` field grants no authority. An admin key does not grant cross-tenant access.

The approval fingerprint includes tenant, principal, session, task, protocol, tool, target, capability, and arguments. Consumption remains atomic and single-use. Concurrent retries can execute an approved action at most once. Failed upstream attempts also consume approval; obtain another approval before retrying an uncertain write.

Use `AEGISFLOW_API_KEY` with `aegisctl` for protected admin commands. The dashboard displays an authentication notice for protected views. It does not collect or persist reviewer keys. `test-action` requires operator role and uses authenticated identity, but remains a policy diagnostic rather than an upstream execution test.

## Persistent state upgrade

Startup expires pending and unconsumed approved MCP entries that lack the new trusted identity binding. This change is persisted and requires those actions to be reviewed again. Already consumed entries retain their consumed state. Valid bound HTTP approvals survive restarts with the same configuration, key, and session label. Signed evidence and approval integrity checks still fail closed on tampering.

Legacy evidence with no tenant, or a chain containing multiple tenants, is unavailable through tenant-scoped APIs. Preserve the database for offline inspection; do not assign historical records a tenant by guesswork. Back up existing state before upgrading a deployed installation.

Downgrades are unsupported: older binaries do not enforce these boundaries. Restoring a pre-upgrade database can restore unbound approvals. Keep backups offline and require fresh review after rollback.

## Local verification

```sh
go test -race ./internal/admin ./internal/mcpgw ./internal/middleware ./internal/approval ./internal/evidence ./cmd/aegisctl
make e2e-approval-security
make e2e-installer
```

Boundary tests exercise unauthorized routes, cross-tenant reads and reviews, same-tenant key isolation, session labels, argument changes, reviewer spoofing, concurrent retries, and SSE ownership. The binary test checks restart, single-use approval, changed arguments, and tampered SQLite state. Installer checks use a disposable checkout and real builds, gateway processes, stdio bridge, and CLI.
