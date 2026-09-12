# AegisFlow v0.9.0: tool boundaries, signed evidence, and verifiable releases

> Archived launch draft for v0.9.0. Do not publish as the development candidate announcement. Runtime upstream credential issuance is disabled; verify release artifacts and version-specific claims before reuse.


AegisFlow v0.9.0 is available.

- Release: https://github.com/saivedant169/AegisFlow/releases/tag/v0.9.0
- Documentation: https://saivedant169.github.io/AegisFlow/
- Local proof: https://saivedant169.github.io/AegisFlow/PR_WRITER/

## What changed

- Supported tool-call translation for OpenAI-compatible, Anthropic, Gemini, and Ollama paths.
- Optional Anthropic Messages API tool passthrough, disabled by default.
- Policy checks for tool definitions, arguments, and MCP tool lists.
- Single-use approval resume based on stable action fields.
- Signed evidence chain per session.
- Upstream credential issuance is unavailable in the development candidate.
- Streaming output checks before bytes reach client.
- SHA-256 installer verification, Sigstore bundle, SPDX SBOM, and build provenance.

Full notes: https://github.com/saivedant169/AegisFlow/blob/v0.9.0/docs/releases/v0.9.0.md

## Proof

Recorded local flow uses running AegisFlow MCP and admin endpoints against mock GitHub upstream:

1. Repository list is allowed.
2. Repository deletion is blocked.
3. Pull request creation waits for review.
4. Reviewer approves exact action.
5. Retry reaches upstream.
6. Signed evidence chain verifies.

![AegisFlow governed pull request proof](https://raw.githubusercontent.com/saivedant169/AegisFlow/v0.9.0/docs/assets/hero-pr-writer.gif)

## Try it

```bash
git clone https://github.com/saivedant169/AegisFlow.git
cd AegisFlow/starter-kit
./install-pr-writer.sh
```

Go 1.26.6 or later, Node.js, `curl`, and `jq` are required.

## Boundary

AegisFlow governs traffic routed through configured endpoint. It does not sandbox agent process and cannot stop direct calls that bypass gateway. Built-in editor file and shell actions need explicit routing through MCP or another supported boundary.

Please post policy examples, setup problems, or boundary questions in Discussions. For security reports, follow [SECURITY.md](https://github.com/saivedant169/AegisFlow/blob/main/SECURITY.md).
