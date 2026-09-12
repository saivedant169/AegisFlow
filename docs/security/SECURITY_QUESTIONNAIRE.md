# Security questionnaire: development candidate

These answers describe the current development source, not a certification or the published v0.9.0 binary. Use [runtime support and verification](../runtime-support.md) and [candidate migration](../releases/corrective-candidate.md) for version boundaries.

## What does the gateway enforce?

Configured MCP endpoints authenticate callers, bind sessions to identity, evaluate tool policy, and require matching approval before reviewed calls can dispatch. Model API endpoints apply input and output policy. Built-in editor file and shell operations that bypass these endpoints remain outside coverage.

## Does it isolate agents or secrets?

The gateway is not a process sandbox. An agent running as the same operating-system user may read that user's files or environment. Protect provider secrets and signing keys with separate process identities and deployment isolation. Runtime upstream credential issuance is disabled; broker libraries are not an available mitigation.

## What survives restart?

Memory mode does not persist approval or evidence state. SQLite mode requires a retained database and stable signing key. Test restart, replay, and tampered-state rejection with `scripts/e2e_approval_security.sh`. See [identity migration](../mcp-identity.md).

## What does evidence prove?

HMAC signatures and hash links detect modifications to retained records without the key. They do not provide public-key non-repudiation. A signing-key holder can forge evidence. Missing final records or omitted sessions require an independent checkpoint or export to detect. Evidence cannot attest to actions that bypassed the gateway.

## What happens when storage fails?

The routed MCP path fails closed when required evidence or approval state cannot be persisted. This does not make upstream side effects transactional with local storage. After dispatch, a failed response or outcome write can leave execution uncertain; do not retry a write automatically without checking upstream state.

## Is least privilege automatic?

No. Configure narrow upstream permissions, caller roles, policy rules, and network access. An allow decision cannot reduce the permissions of an existing upstream token. Keep the admin service private and use TLS outside the local demo.

## Which security checks are available?

Repository checks include race tests, CLI E2E, approval restart/replay/tamper E2E, installer tests, vulnerability scanning, and container smoke tests. Their passing results apply to the tested source and environment, not every production deployment. Hosted release signing and provenance require a separate release workflow.

## Is regulatory compliance certified?

No certification, customer incident reduction, or guaranteed loss prevention is claimed here. Operators must assess their own deployment, data retention, access controls, and incident procedures. See [threat model](THREAT_MODEL.md) and [production checklist](../production-checklist.md).
