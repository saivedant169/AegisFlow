# Operations Runbook

This runbook covers the free, local-first operating path first. Optional production components are documented separately and should only be enabled when a user chooses to run them.

## Local Operating Mode

Start the local demo:

```bash
make demo-local
```

Start from source with the mock provider:

```bash
make build
./bin/aegisflow --config examples/configs/single-tenant.yaml
```

Verify health:

```bash
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/health
```

Verify a mock chat request:

```bash
./examples/requests/openai-compatible-curl.sh
```

## Cost-Free Defaults

The default local path uses:

- mock provider
- YAML policies
- in-memory rate limiting
- local audit/evidence chain
- Prometheus metrics on the admin port
- Docker Compose for local services

Real providers, hosted tracing, Redis, PostgreSQL, Kubernetes, and external policy engines are optional.

## Metrics

Prometheus metrics are available on the admin port:

```bash
curl -fsS http://localhost:8081/metrics | grep '^aegisflow_'
```

Recommended local checks:

- `aegisflow_requests_total`
- `aegisflow_request_duration_seconds`

See [observability.md](observability.md) for the included Prometheus scrape config and Grafana dashboard JSON.

## Audit And Evidence

Verify the audit chain:

```bash
curl -fsS -X POST http://localhost:8081/admin/v1/audit/verify \
  -H "X-API-Key: demo-key-001"
```

List evidence sessions:

```bash
curl -fsS http://localhost:8081/admin/v1/evidence/sessions \
  -H "X-API-Key: demo-key-001"
```

Export a session:

```bash
./bin/aegisctl evidence export <session-id> --file evidence.json
./bin/aegisctl evidence report <session-id> --file evidence.md
```

## Built-In Snapshot Backups

When `resilience.enabled` is true, the admin API can create and list local JSON snapshots. This is useful for lightweight local state capture.

Enable resilience with a config such as `configs/resilience-example.yaml`, then create a snapshot:

```bash
curl -fsS -X POST http://localhost:8081/admin/v1/resilience/backup \
  -H "X-API-Key: demo-key-001"
```

List snapshots:

```bash
curl -fsS http://localhost:8081/admin/v1/resilience/backups \
  -H "X-API-Key: demo-key-001"
```

Snapshots are stored under the configured `resilience.backup_dir`.

## PostgreSQL Backup

PostgreSQL is optional. Use it when audit, evidence, and usage records need to survive restarts.

Create a dump from the Docker Compose database:

```bash
docker compose -f deployments/docker-compose.yaml exec postgres \
  pg_dump -U aegisflow -d aegisflow -Fc -f /tmp/aegisflow.dump

docker compose -f deployments/docker-compose.yaml cp \
  postgres:/tmp/aegisflow.dump ./aegisflow.dump
```

Restore into a fresh local database:

```bash
docker compose -f deployments/docker-compose.yaml cp \
  ./aegisflow.dump postgres:/tmp/aegisflow.dump

docker compose -f deployments/docker-compose.yaml exec postgres \
  pg_restore -U aegisflow -d aegisflow --clean --if-exists /tmp/aegisflow.dump
```

After restore:

```bash
curl -fsS -X POST http://localhost:8081/admin/v1/audit/verify \
  -H "X-API-Key: demo-key-001"
```

## Incident Response

If a policy allows something unexpected:

1. Stop the local stack or remove gateway ingress.
2. Export audit and evidence records.
3. Verify the audit/evidence chain.
4. Tighten the relevant YAML policy.
5. Add or update a regression example.
6. Restart with the updated config and replay the blocked request.

## Upgrade Checks

Before upgrading:

```bash
make fmt-check
go test ./... -race -count=1
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
bash scripts/compose_smoke.sh
```

After upgrading:

```bash
curl -fsS http://localhost:8080/health
curl -fsS http://localhost:8081/metrics | grep '^aegisflow_'
```
