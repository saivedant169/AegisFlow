# Troubleshooting

If something is wrong, start here. Each entry shows the symptom, the verbatim error you will see, and the fix.

For setup help that is not covered here, open an [install problem](https://github.com/saivedant169/AegisFlow/issues/new?template=install_problem.yml). For unexpected runtime behavior, open a [bug report](https://github.com/saivedant169/AegisFlow/issues/new?template=bug_report.yml).

---

## Port already in use

**Symptom:** `aegisflow` exits immediately on startup, or `docker compose up` reports a bind failure.

**Verbatim error:**

```
listen tcp 0.0.0.0:8080: bind: address already in use
```

**Likely causes**

- Another AegisFlow process is already running (`pgrep aegisflow`).
- A previous `docker compose` stack was not torn down (`docker compose ls`).
- A different service holds the port (`lsof -i :8080`).

**Fix**

```bash
# Find what owns the port
lsof -i :8080
lsof -i :8081
lsof -i :8082

# Stop a stale compose stack
cd starter-kit/deploy && docker compose down

# Or run AegisFlow on a different port
./bin/aegisflow --config configs/aegisflow.yaml \
  --port 18080 --admin-port 18081 --metrics-port 18082
```

---

## Docker daemon not running

**Symptom:** the starter-kit installer exits with a connection error before AegisFlow ever starts.

**Verbatim error:**

```
Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
```

**Fix**

- macOS: open Docker Desktop, wait for the whale icon to stop animating.
- Linux: `sudo systemctl start docker`
- Verify: `docker ps` should succeed.

If you do not want Docker at all, build the binary directly:

```bash
make build
./bin/aegisflow --config configs/demo.yaml
```

The binary path requires neither Docker nor Postgres for the mock-provider demo.

---

## Missing or wrong GitHub App config

**Symptom:** policy says `allow`, AegisFlow tries to mint a GitHub App JWT, and the request fails after the policy check.

**Verbatim errors you may see**

```
github: app credentials not configured (set GITHUB_APP_ID, GITHUB_APP_INSTALLATION_ID, GITHUB_APP_PRIVATE_KEY_PATH)
```

```
github: invalid private key: x509: failed to parse PKCS1 private key
```

```
github: 401 Unauthorized — Bad credentials
```

**Fix checklist**

- [ ] `GITHUB_APP_ID` is the **numeric** App ID, not the client ID.
- [ ] `GITHUB_APP_INSTALLATION_ID` matches the installation that owns the target repo.
- [ ] `GITHUB_APP_PRIVATE_KEY_PATH` points at the `.pem` you downloaded from the App settings. Permissions should be `600`.
- [ ] The App has `pull_requests:write`, `contents:read`, and `metadata:read` at minimum.
- [ ] The installation is on the repo or org you are targeting (re-check `https://github.com/settings/installations`).

Test the credentials without going through the agent:

```bash
aegisctl status                 # admin reachable + no chain errors
curl -s http://localhost:8081/admin/v1/providers -H "X-API-Key: $ADMIN_KEY"
```

---

## Invalid policy file

**Symptom:** `aegisflow` either refuses to start or starts but every action gets the default decision (usually `review` or `block`) regardless of your rules.

**Verbatim errors you may see**

```
failed to load config: parsing config file: yaml: line 23: did not find expected key
```

```
[config] tool policy rule #4 references unknown protocol "bogus", rule will never match
```

**Fix**

1. **Lint the YAML.** Any decent editor will highlight indentation issues. `yq eval . your-config.yaml` exits non-zero on malformed YAML.
2. **Read the startup warnings.** On startup AegisFlow walks `tool_policies.rules` and logs `[config] tool policy rule #N references unknown protocol ...` for anything outside `mcp | http | shell | sql | git | *`. If you see this line, that rule never matches — fix the protocol field.
3. **Confirm the policy is the one you think.** `aegisctl policies` lists what the running process has loaded.
4. **Use `aegisctl simulate` to dry-run an envelope.** Cheaper than restarting the gateway every time you tweak a rule.

```bash
aegisctl simulate \
  --protocol git --tool github.create_pull_request \
  --target your-org/your-repo --capability write
```

---

## Evidence verification confusion

**Symptom:** `aegisctl evidence verify` returns `valid: false`, or you cannot find the session you expected.

**Verbatim outputs**

Healthy:

```
valid: true, total_entries: 7, audit log integrity verified
```

Tampered or partial:

```
valid: false at entry #5 (b1f0c324-…): hash chain broken — recorded prev_hash does not match previous entry
```

**Likely causes and fixes**

| Output / behavior | Cause | Fix |
|-------------------|-------|-----|
| `session not found` | Wrong `--session` ID, or the session is on a different host. | `aegisctl evidence sessions` to list available sessions; copy the exact ID. |
| `valid: false at entry #N` | A row in the evidence store was edited or deleted after the fact. | Pull the immutable backup. The chain is intentionally non-repairable — that's the point. |
| `valid: true` but exported file looks short | You verified the live chain, but exported only the actions visible to your tenant. | Run the export as an admin token, or scope the session correctly. |
| Verify hangs | Admin API unreachable. | `aegisctl status` first. |

Quick health check before you panic:

```bash
aegisctl status --json | jq '.chain_valid'    # should be true
aegisctl evidence verify --session $SID       # explicit per-session verify
```

If `chain_valid: false` shows up under steady-state operation with no operator changes, that is a security event. Treat it as one: snapshot the database, do not let the process keep writing, and file an [install problem](https://github.com/saivedant169/AegisFlow/issues/new?template=install_problem.yml) with the verify output.

---

## Where to look next

- Reverse-proxy setup: [docs/deploy/reverse-proxy.md](deploy/reverse-proxy.md)
- Proof artifact / scenario walkthrough: [docs/PR_WRITER.md](PR_WRITER.md)
- Starter-kit policy packs: [starter-kit/README.md](../starter-kit/README.md)
- Discussions for everything else: <https://github.com/saivedant169/AegisFlow/discussions>
