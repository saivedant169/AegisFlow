# 15-Minute PR-Writer Quickstart

## What you'll do
- Install AegisFlow
- Connect Claude Code
- Watch AegisFlow block a destructive action
- Watch it review a PR creation
- Approve it
- Verify the evidence chain

## Prerequisites
- Go 1.26.2+
- Node.js
- `jq` and `curl`
- Claude Code (or any MCP-compatible agent)

## 1. Install (2 minutes)

```bash
cd starter-kit
./install-pr-writer.sh
```

The installer will:
- Free ports 8080, 8081, 8082, 3000
- Generate `configs/pr-writer.yaml` from the `pr-writer` policy pack
- Build `bin/aegisflow` and `bin/aegisctl`
- Start AegisFlow and the mock MCP server in the background
- Write `.mcp.json` so Claude Code can discover the gateway
- Run 3 sanity checks (health, allow, block)

When it succeeds you'll see:

```
Done. Now open Claude Code in this directory and type:
  'list my github repos using aegisflow tools'
```

## 2. Connect Claude Code (1 minute)

The installer already created `.mcp.json` in the project root. Just open Claude
Code in this directory:

```bash
claude
```

Then verify the gateway is connected:

```
/mcp
```

You should see `aegisflow` listed as a connected MCP server.

## 3. Test the 3 decisions (5 minutes)

### a. ALLOW -- a safe read

In Claude Code:
> list my github repos using aegisflow tools

The mock returns a list of repos. AegisFlow logs an `allow` decision because
`github.list_*` is allow-listed in `pr-writer.yaml`.

### b. REVIEW -- creating a PR

> create a pull request titled "demo: add hello world" against main using aegisflow tools

AegisFlow returns a `review` decision and parks the call as a pending approval.
Claude Code will appear to wait. Approve it with:

```bash
curl -s http://localhost:8081/admin/v1/approvals \
  -H "X-API-Key: pr-writer-key-001" | jq .

# Find the id, then:
curl -X POST http://localhost:8081/admin/v1/approvals/<id>/approve \
  -H "X-API-Key: pr-writer-key-001"
```

The agent unblocks and the PR mock returns success.

### c. BLOCK -- a destructive action

> delete the staging repo using aegisflow tools

AegisFlow blocks immediately. The decision is logged. The agent gets a clean
"action denied by policy" error -- no destructive call ever leaves the
gateway.

You can also test these without Claude Code:

```bash
# Allowed
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "X-API-Key: pr-writer-key-001" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"mcp","tool":"github.list_repos","capability":"read"}' | jq .

# Blocked
curl -s -X POST http://localhost:8081/admin/v1/test-action \
  -H "X-API-Key: pr-writer-key-001" \
  -H "Content-Type: application/json" \
  -d '{"protocol":"mcp","tool":"github.delete_repo","capability":"delete"}' | jq .
```

## 4. Verify evidence (2 minutes)

Every decision is recorded. Pull the recent decision log:

```bash
curl -s http://localhost:8081/admin/v1/requests \
  -H "X-API-Key: pr-writer-key-001" | jq '.[0:5]'

curl -s http://localhost:8081/admin/v1/violations \
  -H "X-API-Key: pr-writer-key-001" | jq .
```

You should see:
- An `allow` for `github.list_repos`
- A `review` (then `approve`) for `github.create_pull_request`
- A `block` for `github.delete_repo`

That's the evidence chain: every tool call your agent attempted, what
AegisFlow decided, why, and (for reviews) who approved it.

## 5. Tune the policy (5 minutes)

The active policy lives in `configs/pr-writer.yaml`. The `tool_policies` block
is what controls decisions. To make a change:

1. Edit `configs/pr-writer.yaml` (or `starter-kit/policies/pr-writer.yaml` and
   rerun the installer to regenerate).
2. Find the rule, e.g.:
   ```yaml
   - protocol: "mcp"
     tool: "github.create_pull_request"
     decision: "review"
   ```
3. Change `review` -> `allow` to auto-approve PR creation, or `review` ->
   `block` to deny it outright.
4. Restart AegisFlow:
   ```bash
   ./starter-kit/uninstall-pr-writer.sh
   ./starter-kit/install-pr-writer.sh
   ```

Common knobs:
- Allow more reads: add `github.search_*` patterns
- Tighten branch protection: set `github.create_branch` to `review`
- Permit merges (not recommended): set `github.merge_pull_request` to `review`

## Cleanup

```bash
./starter-kit/uninstall-pr-writer.sh
```

This kills both background processes and removes `.mcp.json`.
