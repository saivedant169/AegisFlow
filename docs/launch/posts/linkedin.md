# LinkedIn post

> Archived launch draft for v0.9.0. Do not publish as the development candidate announcement. Runtime upstream credential issuance is disabled; verify release artifacts and version-specific claims before reuse.


Coding agents now make privileged calls inside development environments. Logging result is useful, but it does not stop destructive call.

I built AegisFlow to put policy before execution on configured MCP and model paths. Routed actions become structured envelopes. Policy can allow, hold for review, or block.

v0.9.0 proof runs against local mock GitHub upstream. Repository reads pass. Repository deletion is blocked before upstream. Pull request creation waits in approval queue. Exact approved retry passes. Signed session evidence verifies.

This release also adds supported tool-call translation, per-session evidence, checksum verification, Sigstore bundle, SBOM, and build provenance.

Important limit: AegisFlow is policy gateway, not process sandbox. Calls that bypass configured endpoint stay outside policy.

Proof: https://saivedant169.github.io/AegisFlow/PR_WRITER/

Repository: https://github.com/saivedant169/AegisFlow

Attach: `docs/assets/hero-pr-writer.gif`, `docs/assets/shot-approval-queue.png`
