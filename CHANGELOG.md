# Changelog

All notable changes to AegisFlow will be documented in this file.

## [0.1.0] - 2024-03-24

### Added
- Unified AI gateway with OpenAI-compatible API
- Provider adapters: Mock, OpenAI, Anthropic, Ollama
- Intelligent routing with glob-based model matching
- Priority and round-robin routing strategies
- Automatic fallback with circuit breaker
- In-memory rate limiting (sliding window)
- Redis-backed rate limiting (optional)
- Tenant authentication via API keys
- Policy engine with input and output checks
- Keyword blocklist filter
- Regex pattern filter
- PII detection (email, SSN, credit card)
- Usage tracking with per-tenant, per-model aggregation
- Cost estimation per request
- OpenTelemetry tracing (stdout and OTLP exporters)
- Prometheus metrics endpoint
- Structured JSON logging
- Admin API with health, metrics, and usage endpoints
- Docker and Docker Compose support
- GitHub Actions CI/CD
- Comprehensive test suite
