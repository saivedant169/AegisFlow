# AegisFlow — benchmark card

A single shareable card of the real numbers. Reproduce locally with
`./scripts/run_benchmarks.sh` and `go run ./scripts/benchmark_governance.go`.

```
┌─────────────────────────────────────────────────────────────┐
│  AegisFlow — governance at the agent/tool boundary          │
├─────────────────────────────────────────────────────────────┤
│  Governance pipeline       58,000+ evals/s                    │
│  Latency  p50 / p95 / p99  1.1 ms / 4.2 ms / 7.3 ms         │
│                                                             │
│  Governance decision cost (Apple M1, micro-benchmarks):     │
│    envelope creation             ~0.4 µs                    │
│    policy evaluate (allow)        ~1.2 µs                    │
│    policy + evidence chain        ~3.4 µs                    │
│    full allow (+ credential)      ~5.2 µs                    │
│                                                             │
│  The decision itself is single-digit microseconds.         │
│  ~80% test coverage · Go single binary · Apache-2.0         │
└─────────────────────────────────────────────────────────────┘
```

| Metric | Value |
|--------|-------|
| Governance pipeline throughput | 58,000+ evals/sec |
| p50 latency | 1.1 ms |
| p95 latency | 4.2 ms |
| p99 latency | 7.3 ms |
| Envelope creation | ~0.4 µs |
| Policy evaluate (allow, 20 rules) | ~1.2 µs |
| Policy + evidence chain | ~3.4 µs |
| Full allow (policy + evidence + credential) | ~5.2 µs |

Numbers are micro-benchmarks on an Apple M1 (8 GB RAM); the governance
overhead is the cost added on top of forwarding a request. See
[docs/PR_WRITER.md](../PR_WRITER.md) for the costed end-to-end walkthrough.
