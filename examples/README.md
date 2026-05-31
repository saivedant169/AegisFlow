# Examples

These examples are designed to run locally without paid services, provider keys, or hosted infrastructure.

## Config Examples

Run any config with the local binary:

```bash
make build
./bin/aegisflow --config examples/configs/single-tenant.yaml
```

Then send requests from another terminal:

```bash
./examples/requests/openai-compatible-curl.sh
```

Available configs:

| File | Purpose |
|------|---------|
| `examples/configs/single-tenant.yaml` | One tenant, one key, mock provider |
| `examples/configs/multi-tenant.yaml` | Two tenants with separate keys and limits |
| `examples/configs/policy-blocking.yaml` | Mock provider with blocking input policies |
| `examples/configs/webhook-local.yaml` | Mock provider with local signed webhook events |

## Request Examples

`examples/requests/openai-compatible-curl.sh` shows the OpenAI-compatible request shape using the mock provider. It uses `Authorization: Bearer <key>` so it works with clients that expect that authentication pattern.

No request in this directory calls a paid provider.

## CLI Automation

`examples/cli-automation.sh` shows how to drive AegisFlow from scripts and CI with the machine-readable CLI surface:

```bash
./examples/cli-automation.sh           # safe: reports only
APPROVE=1 ./examples/cli-automation.sh # also acts on the first pending item
```

It covers three patterns:

- **Test a policy without executing it** — `aegisctl test-action --dry-run` evaluates the policy locally and prints the decision. No admin API call, no audit entry, no approval queued. Use it while iterating on a policy pack.
- **Parse pending approvals programmatically** — `aegisctl pending --json` emits the raw array; pipe it into `jq` to script approve/deny logic.
- **Gate a pipeline on health** — `aegisctl status --json` exits `0` when healthy, `1` when the gateway/admin is down or the evidence chain is invalid.

```bash
# dry-run a risky tool: see the decision, change nothing
aegisctl test-action --dry-run --protocol mcp --tool github.delete_repo --target acme/widgets

# count pending items in CI
aegisctl pending --json | jq 'length'

# fail a deploy step if AegisFlow is unhealthy
aegisctl status --json | jq -e '.healthy == true'
```

## Webhook Example

Run the local webhook sink:

```bash
python3 examples/webhook/local_sink.py
```

Then run AegisFlow with:

```bash
make build
./bin/aegisflow --config examples/configs/webhook-local.yaml
```

See `docs/webhooks.md` for the full flow.
