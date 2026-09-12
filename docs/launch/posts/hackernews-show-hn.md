# Show HN: AegisFlow, local policy gateway for coding agents and MCP tools

> Archived launch draft for v0.9.0. Do not publish as the development candidate announcement. Runtime upstream credential issuance is disabled; verify release artifacts and version-specific claims before reuse.


I built AegisFlow because coding agents now call tools with real side effects, while most setups still hand them standing credentials and review results after execution.

AegisFlow is an Apache-2.0 Go service that sits on configured MCP, OpenAI-compatible, or Anthropic Messages API path. Routed tool calls become an `ActionEnvelope` with actor, task, protocol, tool, target, arguments, and requested capability. Policy returns `allow`, `review`, or `block` before upstream execution.

v0.9.0 adds tool-call translation across supported provider adapters, optional Messages API tool passthrough, one signed evidence chain per session, scoped GitHub App and AWS STS requests, and single-use approval resume. Release artifacts include SHA-256 checksums, Sigstore bundle, SPDX SBOM, and GitHub build provenance.

I recorded local proof rather than screen mock. It sends MCP calls through running gateway to mock GitHub upstream:

- `github.list_repos` is allowed and reaches upstream.
- `github.delete_repo` is blocked with JSON-RPC `-32001`.
- `github.create_pull_request` waits in approval queue.
- Reviewer approves exact action, client retries, and call reaches upstream.
- Signed evidence chain verifies.

Proof: https://saivedant169.github.io/AegisFlow/PR_WRITER/

On Apple M1, zero-latency mock HTTP test handled 30,000 requests at 55,327 req/s with 0.6 ms p50 and 3.6 ms p99. With cache disabled and 25 ms mock provider delay, result was 624.57 req/s, 28.0 ms p50, 35.0 ms p99, 0.00% errors. Scripts and limits are documented: https://saivedant169.github.io/AegisFlow/performance/

Important boundary: AegisFlow governs traffic routed through it. It is not process sandbox. Built-in editor file or shell tools are outside policy unless routed through MCP or another configured boundary. Messages API tool passthrough is disabled by default until operator tests provider loop.

Repository: https://github.com/saivedant169/AegisFlow

I would value feedback from people running coding agents against real repositories: which actions belong in review rather than block, and where does gateway boundary fail to fit your workflow?
