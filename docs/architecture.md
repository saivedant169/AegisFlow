# AegisFlow Architecture

## System Overview

AegisFlow is a single Go binary that acts as a reverse proxy, policy boundary, and control plane for AI/LLM traffic and tool-using agents. It intercepts requests between clients and providers, adding authentication, rate limiting, policy enforcement, routing, usage tracking, approvals, evidence, and observability.

The default path is fully local and cost-free: a mock provider, YAML policies, in-memory storage, local audit evidence, and Prometheus metrics. Real providers, Redis, PostgreSQL, Kubernetes, and external policy systems are optional integrations.

## Local-First Runtime

```mermaid
graph LR
    App[App or coding agent] --> Gateway[AegisFlow gateway]
    Gateway --> Auth[API key auth]
    Auth --> Policy[YAML policy engine]
    Policy --> Router[Router]
    Router --> Mock[Mock provider]
    Router -. optional .-> OpenAI[OpenAI-compatible provider]
    Router -. optional .-> Anthropic[Anthropic provider]
    Router -. optional .-> Azure[Azure OpenAI provider]
    Gateway --> Audit[Local audit and evidence]
    Gateway --> Metrics[Prometheus metrics]
    Gateway --> Admin[Admin API and dashboard]
```

Nothing in the local path requires a hosted account. Optional providers only run when a user configures their own key.

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

    Router -. optional .-> OpenAI[OpenAI API]
    Router -. optional .-> Anthropic[Anthropic API]
    Router --> Ollama[Ollama]
    Router --> Mock[Mock Provider]

    Usage --> Metrics[Prometheus /metrics]
    Usage --> Traces[OTel Traces]
    Usage --> Admin[Admin API :8081]
    Usage --> Audit[Audit / evidence]
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

**Mock provider first.** The mock provider is a real first-class route target. It keeps demos, tests, CI, and local development free and repeatable.

**Optional external services.** Paid providers, cloud secret managers, hosted tracing backends, Redis, and PostgreSQL are optional. The project must remain useful without them.

**Middleware chain.** Each cross-cutting concern (auth, rate limiting, logging, metrics) is an independent middleware that can be added or removed from the chain.

**In-memory by default, Redis optional.** Rate limiting works without any external dependencies. Redis is available for distributed deployments.

**Circuit breaker per provider.** Failed providers are temporarily removed from the routing pool to prevent cascading failures.
