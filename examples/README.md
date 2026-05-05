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
