# Getting Started with AegisFlow

## Prerequisites

- Go 1.26.2+ (for building from source)
- Docker and Docker Compose (for containerized deployment)

## Quick Start

### Option 1: One-Command Local Demo

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow
make demo-local
```

This runs with the mock provider and does not require any paid service or provider key.

### Option 2: Build from Source

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow

# Build
make build

# Run with default config (mock provider enabled)
make run
```

### Option 3: Docker Compose

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow

docker compose -f deployments/docker-compose.yaml up --build
```

## Your First Request

AegisFlow ships with a mock provider enabled by default, so you can start making requests immediately without any API keys.

```bash
# Check the gateway is running
curl http://localhost:8080/health

# Send a chat completion request
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "X-API-Key: aegis-test-default-001" \
  -d '{
    "model": "mock",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

## Using with the OpenAI Python SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="aegis-test-default-001"
)

response = client.chat.completions.create(
    model="mock",
    messages=[{"role": "user", "content": "Hello from Python!"}]
)

print(response.choices[0].message.content)
```

## Connecting a Real Provider

Real providers are optional. The mock provider remains the default path for free demos, local development, and CI.

Edit `configs/aegisflow.yaml` to enable a provider:

```yaml
providers:
  - name: "openai"
    type: "openai"
    enabled: true
    base_url: "https://api.openai.com/v1"
    api_key_env: "OPENAI_API_KEY"
    models:
      - "openai-chat"
      - "openai-fast"
```

Set your API key and restart:

```bash
export OPENAI_API_KEY="sk-..."
make run
```

Now requests for configured OpenAI-backed models will route to OpenAI with automatic fallback to the mock provider.

## Monitoring

- **Prometheus metrics**: `http://localhost:8081/metrics`
- **Usage statistics**: `http://localhost:8081/admin/v1/usage`
- **Health check**: `http://localhost:8081/health`

## Running the Demo

```bash
make demo-local
```

This exercises all major features: health check, chat completion, streaming, policy blocking, auth, and usage tracking.

## Local Example Configs

The `examples/configs` directory includes copy-pasteable setups that use only the mock provider:

- `single-tenant.yaml`
- `multi-tenant.yaml`
- `policy-blocking.yaml`

Run one:

```bash
make build
./bin/aegisflow --config examples/configs/single-tenant.yaml
./examples/requests/openai-compatible-curl.sh
```

## Next Steps

- [Architecture documentation](architecture.md)
- [Observability guide](observability.md)
- [Webhook guide](webhooks.md)
- [Operations runbook](operations-runbook.md)
- [Configuration reference](../configs/aegisflow.example.yaml)
- [API specification](../api/openapi.yaml)
- [Contributing guide](../CONTRIBUTING.md)
