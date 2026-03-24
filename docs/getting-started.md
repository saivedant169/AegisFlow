# Getting Started with AegisFlow

## Prerequisites

- Go 1.24+ (for building from source)
- Docker and Docker Compose (for containerized deployment)

## Quick Start

### Option 1: Build from Source

```bash
git clone https://github.com/aegisflow/aegisflow.git
cd aegisflow

# Build
make build

# Run with default config (mock provider enabled)
make run
```

### Option 2: Docker Compose

```bash
git clone https://github.com/aegisflow/aegisflow.git
cd aegisflow

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
  -H "X-API-Key: aegis-dev-key-001" \
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
    api_key="aegis-dev-key-001"
)

response = client.chat.completions.create(
    model="mock",
    messages=[{"role": "user", "content": "Hello from Python!"}]
)

print(response.choices[0].message.content)
```

## Connecting a Real Provider

Edit `configs/aegisflow.yaml` to enable a provider:

```yaml
providers:
  - name: "openai"
    type: "openai"
    enabled: true
    base_url: "https://api.openai.com/v1"
    api_key_env: "OPENAI_API_KEY"
    models:
      - "gpt-4o"
      - "gpt-4o-mini"
```

Set your API key and restart:

```bash
export OPENAI_API_KEY="sk-..."
make run
```

Now requests for `gpt-*` models will route to OpenAI with automatic fallback to the mock provider.

## Monitoring

- **Prometheus metrics**: `http://localhost:8081/metrics`
- **Usage statistics**: `http://localhost:8081/admin/v1/usage`
- **Health check**: `http://localhost:8081/health`

## Running the Demo

```bash
chmod +x scripts/demo.sh
./scripts/demo.sh
```

This exercises all major features: health check, chat completion, streaming, policy blocking, auth, and usage tracking.

## Next Steps

- [Architecture documentation](architecture.md)
- [Configuration reference](../configs/aegisflow.example.yaml)
- [API specification](../api/openapi.yaml)
- [Contributing guide](../CONTRIBUTING.md)
