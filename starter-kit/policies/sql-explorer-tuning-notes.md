# Tuning notes: sql-explorer

## When to use this pack

Give a coding or BI agent **read-first** access to a database so it can answer
questions and explore schemas, without the ability to mutate or destroy data.
Typical users: data analysts, BI assistants, ad-hoc analytics, schema
discovery during onboarding.

If the agent only ever needs to read (never write), use `readonly.yaml`
instead. If it needs to write code/PRs, use `pr-writer.yaml`.

## Decision model

| Class | Decision | Why |
|-------|----------|-----|
| `sql.select` | allow | Reads are the whole point. |
| `sql.insert`, `sql.update` | review | A human confirms scope before a write lands. |
| `sql.delete`, `sql.drop_*`, `sql.truncate`, `sql.grant`, `sql.revoke` | block | Destructive or privilege-changing; never automatic. |
| `github.list_*`, `github.get_*` | allow | Find schema/migration files in a repo. |
| GitHub writes | block | Out of scope for a data explorer. |
| Read-only shell (`ls`, `cat`, `grep`, `find`, `head`, `tail`, `wc`) | allow | Inspect files/schemas locally. `cat` of `.env` / `/etc/shadow` is blocked first. |
| Everything else | block | `default_decision: block`. |

## Known limitations

- **WHERE-clause enforcement is not expressible here.** Rules match on
  `protocol` + `tool` (+ optional `target` glob), not on SQL body. So
  "UPDATE/DELETE must have a WHERE clause" cannot be enforced by this pack;
  that is why `sql.insert`/`sql.update` go to **review** rather than allow, and
  `sql.delete` is **blocked** outright. If your tooling normalizes SQL into a
  tool name that encodes the predicate, add a more specific rule above the
  generic one (first match wins).
- **Tool naming depends on your SQL adapter.** This pack assumes tools named
  `sql.select`, `sql.insert`, `sql.update`, `sql.delete`, `sql.drop_*`,
  `sql.truncate`, `sql.grant`, `sql.revoke`. If your MCP/SQL bridge emits
  different names, rename the rules to match — the decisions are the contract,
  the names are just keys.

## Tightening / loosening

- **Tighten:** move `sql.insert`/`sql.update` from `review` to `block` for a
  pure read-only analyst.
- **Loosen:** if a trusted batch job needs `sql.insert` without a human in the
  loop, scope it by `target` (e.g. a staging schema) and set that specific
  rule to `allow` above the generic `review` rule.
- Add a `target` glob to `sql.select` to restrict which schemas/tables are
  readable (e.g. block `sql.select` on `*.pii_*` before the generic allow).

## Verifying

```bash
# Against a running server loaded with this pack:
aegisctl test-action --protocol sql --tool sql.select   --target prod   # allow
aegisctl test-action --protocol sql --tool sql.drop_table --target prod # block
aegisctl test-action --protocol sql --tool sql.update   --target prod   # review
```

The pack's decisions are also locked by a unit test
(`internal/toolpolicy/sqlexplorer_pack_test.go`) that loads this YAML and
asserts each decision, so changes that break the contract fail CI.
