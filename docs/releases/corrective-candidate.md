# Corrective candidate: development contract

These changes are prepared for v0.9.1. The candidate is not a published release. See [prepared release notes](v0.9.1.md); publication still requires matching hosted artifacts and release checks.

## Credential issuance is unavailable

The gateway now rejects `credentials.enabled: true` at configuration load and no longer constructs runtime credential brokers. Broker libraries remain available for development and tests; they are not a supported credential issuance service in this candidate.

Before upgrading a broker-enabled deployment:

1. Stop workloads that depend on dynamically issued credentials.
2. Revoke or rotate previously issued credentials at the upstream provider. Disabling the local broker does not revoke existing provider tokens.
3. Set `credentials.enabled: false` or remove the credentials block. Remove unused provider secrets from the runtime configuration.
4. Restart and verify the expected allow, review, and block behavior. Do not substitute broad permanent credentials to imitate the disabled broker.

Authentication keys for agents and reviewers remain supported. Those keys authenticate requests to the gateway; they are distinct from optional upstream credential issuance.

## Persistence and client behavior

The published v0.9.0 notes describe a memory-only session registry. The development branch supports optional SQLite persistence and stronger identity binding. Follow [MCP identity and migration](../mcp-identity.md) before upgrading. A stable signing key alone does not persist sessions. Downgrading persisted state to older binaries is unsupported.

The CLI now returns nonzero for invalid verification and failed remote reads, rejects redirects, and requires explicit selection of local example rules. See the [CLI guide](../cli.md). No demonstration credential is supplied implicitly.

## Release preparation

Choose a stable tag only after the release gate passes. Keep these files consistent:

- One dated version heading in `CHANGELOG.md`.
- Matching `version` and `appVersion` in the Helm chart.
- A release note file named `docs/releases/vMAJOR.MINOR.PATCH.md`, beginning with the matching release title.

The release workflow validates this metadata before building. Binary versions, source SBOM filename, release title, release notes, checksums, and provenance paths derive from the validated tag. Existing release assets are not overwritten. Prerelease tags are deliberately rejected by this workflow and installer until a separate prerelease policy is defined.

```sh
python3 scripts/release_metadata.py vMAJOR.MINOR.PATCH
bash scripts/build_release.sh vMAJOR.MINOR.PATCH /tmp/aegisflow-candidate
python3 scripts/e2e_release_install.py vMAJOR.MINOR.PATCH /tmp/aegisflow-candidate
```

The installation E2E uses locally compiled artifacts and a local download fixture. Hosted signing, SBOM generation, and provenance publication still require the hosted release workflow.

## Install a selected binary

For a future published stable tag, download its installer, review it, and run with an explicit version. Substitute the actual published tag:

```sh
AEGISFLOW_VERSION=vMAJOR.MINOR.PATCH sh scripts/install.sh
AEGISFLOW_VERSION=vMAJOR.MINOR.PATCH AEGISFLOW_BINARY=aegisctl sh scripts/install.sh
```

The installer resolves `latest` once through the release API, then downloads the selected binary and checksums from that tag. It checks the checksum and embedded version before replacing an existing installation. Checksum verification detects mismatched artifacts; it does not replace independent signature/provenance verification. Existing v0.9.0 release notes retain their historical behavior and should not be read as this candidate's contract.

## Remaining release gate

These changes do not certify release readiness. Full repository lint, complete claim reconciliation, and exact-candidate release checks remain required. Credential issuance cannot be re-enabled until its identity, scope, revocation, and failure-handling requirements are implemented and tested.

## Plugin file updates

Plugin configuration updates preserve existing file permission bits. Updates reject symlink and other non-regular destinations instead of silently replacing them. If your plugin config is a symlink, use the actual regular config file path where the command supports it, or manage that configuration separately. Plugin removal reports an error before deleting the plugin binary when config persistence fails.
