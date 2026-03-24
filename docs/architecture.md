# AegisFlow Architecture

## System Overview

AegisFlow is a single Go binary that acts as a reverse proxy and control plane for AI/LLM traffic. It intercepts requests between client applications and AI providers, adding authentication, rate limiting, policy enforcement, routing, usage tracking, and observability.

## Component Diagram

```mermaid
graph TB
    Client[Client / OpenAI SDK] -->|HTTP| Gateway

    subgraph AegisFlow
        Gateway[HTTP Server]
        Auth[Auth Middleware]
        RL[Rate Limiter]
        PolicyIn[Policy Engine - Input]
        Router[Router]
        PolicyOut[Policy Engine - Output]
        Usage[Usage Tracker]

        Gateway --> Auth --> RL --> PolicyIn --> Router
        Router --> PolicyOut --> Usage
    end

    Router --> OpenAI[OpenAI API]
    Router --> Anthropic[Anthropic API]
    Router --> Ollama[Ollama]
    Router --> Mock[Mock Provider]

    Usage --> Metrics[Prometheus /metrics]
    Usage --> Traces[OTel Traces]
    Usage --> Admin[Admin API :8081]
```

## Request Flow

```mermaid
sequenceDiagram
    participant C as Client
    participant G as Gateway
    participant A as Auth
    participant R as Rate Limiter
    participant P as Policy Engine
    participant RT as Router
    participant PR as Provider
    participant U as Usage Tracker

    C->>G: POST /v1/chat/completions
    G->>A: Extract API key
    A-->>G: Tenant identified

    G->>R: Check rate limit
    R-->>G: Allowed

    G->>P: Check input policies
    P-->>G: Clean (no violations)

    G->>RT: Route to provider
    RT->>PR: Forward request
    PR-->>RT: Response

    G->>P: Check output policies
    P-->>G: Clean

    G->>U: Record usage + cost
    G-->>C: Return response
```

## Package Structure

| Package | Responsibility |
|---------|---------------|
| `cmd/aegisflow` | Entry point, dependency wiring |
| `internal/config` | YAML configuration loading |
| `internal/gateway` | HTTP handlers for `/v1/chat/completions` and `/v1/models` |
| `internal/middleware` | Auth, rate limiting, logging, metrics middleware |
| `internal/provider` | Provider interface and adapters (Mock, OpenAI, Anthropic, Ollama) |
| `internal/router` | Model-to-provider routing with strategies and fallback |
| `internal/ratelimit` | Rate limiting (in-memory and Redis) |
| `internal/policy` | Input/output policy engine with keyword, regex, and PII filters |
| `internal/usage` | Token counting, cost estimation, per-tenant usage aggregation |
| `internal/telemetry` | OpenTelemetry initialization |
| `internal/admin` | Admin API server (health, metrics, usage) |
| `pkg/types` | Shared request/response types |

## Key Design Decisions

**Single binary, not microservices.** For the MVP, all functionality runs in one process. The internal package boundaries are clean enough to split later if needed.

**OpenAI-compatible API.** Any application using the OpenAI SDK can connect to AegisFlow by changing `base_url`. This is the most important adoption decision.

**Provider interface.** All providers implement the same 6-method interface. Adding a new provider requires zero changes to the gateway, router, or middleware.

**Middleware chain.** Each cross-cutting concern (auth, rate limiting, logging, metrics) is an independent middleware that can be added or removed from the chain.

**In-memory by default, Redis optional.** Rate limiting works without any external dependencies. Redis is available for distributed deployments.

**Circuit breaker per provider.** Failed providers are temporarily removed from the routing pool to prevent cascading failures.
