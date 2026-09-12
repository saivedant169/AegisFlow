# CLI authentication and failure behavior

This guide describes the development CLI. It does not imply that an existing release contains these changes.

See [candidate migration and installation](releases/corrective-candidate.md) for release-specific restrictions.

## Configure endpoints and credentials

```sh
export AEGISFLOW_GATEWAY_URL=http://localhost:8080
export AEGISFLOW_ADMIN_URL=http://localhost:8081
export AEGISFLOW_API_KEY=your-configured-key

aegisctl pending
aegisctl models
```

Use origins without URL credentials, path prefixes, queries, or fragments. A trailing slash is accepted. The CLI sends the configured key only to `/admin/v1/` paths on the configured admin origin and `/v1/` paths on the configured gateway origin. Health checks receive no API key. Plugin registry and artifact downloads use separate unauthenticated clients.

No demonstration key is supplied implicitly. The `test` and `models` commands use the same environment credential as admin commands. Use a credential assigned the required role on the server; an agent key is not automatically a reviewer key. The PR-writer installer provides a separate reviewer key, described in the [starter kit](../starter-kit/README.md).

Provider listings and several diagnostic admin endpoints remain public on the admin listener. Protected approval and evidence endpoints require authentication. Setting a client key does not change server endpoint permissions. Keep the admin listener private.

## Failures are failures

HTTP errors, rejected redirects, connection failures, and invalid JSON on the covered CLI commands exit nonzero. Failed reads are not displayed as successful empty results. Remote error bodies and request URLs are omitted from HTTP error messages because they may contain credentials or request data.

Redirects are rejected, including redirects within the same origin. Configure the final service origin directly. This avoids replaying approval requests or forwarding credentials to redirected paths.

`aegisctl verify` exits nonzero when the server reports `valid: false`. Missing session values and unknown flags are rejected. This batch does not add JSON output to verification; that work remains in contributor PR 145.

```sh
aegisctl verify --session session-id
```

`status --json` exits nonzero when protected status reads fail or the reported MCP gateway is unreachable, even when public health endpoints respond. Health availability alone does not prove that authenticated operations work.

## Local examples require explicit selection

Remote `simulate` and `test-action` failures no longer fall back to local rules. Use `--dry-run` deliberately:

```sh
aegisctl simulate --dry-run --protocol shell --tool get_item --target fixture
aegisctl test-action --dry-run --protocol shell --tool get_item --target fixture
```

These commands evaluate built-in example rules, not the running server's policy. They do not execute upstream actions, persist evidence, or queue real approvals. Printed local identifiers are illustrative.

## Run the regression checks

```sh
make e2e-cli
```

Requires Go and Python 3. The test builds the CLI, exercises controlled HTTP responses, then builds and launches a disposable local gateway with a mock provider. It checks credential delivery, role denial, failure exit codes, redirect rejection, quoted chat input, and explicit local mode. It does not contact paid providers or create hosted repository actions.
