# Production Checklist

Use this checklist before exposing AegisFlow outside a local demo.

## Configuration

- Set `AEGISFLOW_CONFIG` to the mounted production config path.
- Replace all demo tenant API keys.
- Prefer `key_env` for tenant API keys and inject values from a secret manager.
- Keep provider credentials in environment variables or a secret manager, not literal YAML.
- Keep `policies.governance_mode` set to `governance` unless you are intentionally testing fail-open behavior.

## Kubernetes And Helm

- Store tenant API keys in a Kubernetes Secret and enable `config.tenant.apiKeySecret` in Helm values.
- Do not expose the admin service publicly.
- Put gateway ingress behind TLS.
- Set CPU and memory requests/limits for expected traffic.
- Review pod security context, network policy, and ingress annotations for your cluster.

Example Helm secret wiring:

```yaml
config:
  tenant:
    apiKeySecret:
      enabled: true
      name: aegisflow-tenant-key
      key: api-key
      envName: AEGISFLOW_DEFAULT_API_KEY
```

Create the secret:

```bash
kubectl create secret generic aegisflow-tenant-key \
  --from-literal=api-key='replace-with-a-long-random-key'
```

## Runtime Safety

- Verify `/health` on both gateway and admin ports after deploy.
- Send one allowed request through the gateway.
- Send one known-bad prompt and confirm it returns `403`.
- Verify audit/evidence export for a real session.
- Confirm approval flows work before allowing write or deploy actions.

## Observability

- Scrape `/metrics` from the admin service.
- Send application logs to your central logging system.
- Alert on provider failure rate, policy violations, approval backlog, and budget exhaustion.
- Back up the evidence store if PostgreSQL persistence is enabled.

## CI Gates

- Run `make fmt-check`.
- Run `go test ./... -race -count=1`.
- Run `govulncheck ./...`.
- Run `bash scripts/compose_smoke.sh` before release builds.
