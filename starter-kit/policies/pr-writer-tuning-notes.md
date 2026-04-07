# PR Writer Policy Pack — Tuning Notes

## Philosophy: low friction, high safety

A policy pack that prompts on `git status` is a policy pack that gets disabled.
The goal of `pr-writer.yaml` is the opposite: a developer (or coding agent)
working on a feature branch should feel that AegisFlow **stopped the scary
stuff and mostly stayed out of the way.**

Two rules of thumb drove the tuning:

1. **If a senior engineer would not bat an eye, allow it.** Reads, tests,
   builds, branching, committing — none of these need a human approver.
2. **If a mistake here would page someone at 3am, block or review it.** Force
   pushes to main, prod deploys, dropping tables, deleting repos, editing
   `/etc` — these are the moments AegisFlow earns its keep.

Everything in between (opening a PR, `git push`, `kubectl get`, `INSERT`)
goes to **review**, where a human glances at the action and clicks approve.

## What's allowed (no friction)

| Area    | Allowed                                                            | Why |
|---------|--------------------------------------------------------------------|-----|
| GitHub  | `list_*`, `get_*`, `search_*`                                      | Pure reads. |
| GitHub  | `create_branch`, `create_commit`, `create_or_update_file`          | Branching is not destructive. Blocking it just pushes the agent into shell. |
| Shell   | `ls`, `pwd`, `echo`, `grep`, `find`, `head`, `tail`, `wc`, `diff`, `less`, `cat` | Reading the repo is the job. |
| Shell   | `git status / log / diff / show / blame / branch / checkout / switch / fetch / add / commit / stash` | Local-only git. |
| Shell   | `pytest`, `go test`, `go build`, `cargo test/build/check`, `npm test`, `npm run build`, `make test/build/lint` | Tests and builds prove the change works. |
| SQL     | `SELECT`                                                           | Read-only. |
| HTTP    | `GET`, `HEAD`                                                      | Read-only. |

`cat` is allowed by default, but explicit `block` rules sit in front of it
for `.env`, `*.env`, `/etc/shadow`, `/etc/passwd`, `**/.ssh/*`, and any
`credentials*` file. Order matters in the rule list — secrets are checked
first.

## What's reviewed (one click, human in the loop)

| Area    | Reviewed                                                  | Why |
|---------|-----------------------------------------------------------|-----|
| GitHub  | `create_pull_request` (esp. on `agent/*`, `feat/*`, `fix/*`, `bugfix/*`) | The PR itself is the review surface — a human will look at the diff. |
| GitHub  | `merge_pull_request`, `create_deployment`, `update_branch_protection` | Touches main / prod / repo settings. |
| Shell   | `git push`, `git reset --hard`                            | Leaves the developer's machine. |
| Shell   | `kubectl *`, `terraform plan/apply`, `docker build/push`  | Infra blast radius. |
| Shell   | `npm install`, generic `npm` / `make` / `cargo` targets   | Pulls or runs arbitrary code. |
| SQL     | `INSERT`, `UPDATE`                                        | Mutates data. `sqlgate` already flags `UPDATE` without `WHERE`. |
| HTTP    | `POST`, `PUT`, `PATCH`                                    | Mutates remote state. |

## What's blocked (no override without changing the policy)

| Area    | Blocked                                                          | Why |
|---------|------------------------------------------------------------------|-----|
| GitHub  | `delete_repo`, `delete_branch main/master`, `force_push`, `invite_collaborator` | Catastrophic and irreversible. |
| Shell   | `rm`, `dd`, `mkfs`, `chmod`, `shutdown`, `reboot`, `killall`     | Hardware/filesystem/system-level damage. `shellgate/dangerous.go` also pattern-matches `rm -rf /`, fork bombs, and `> /dev/sda`. |
| Shell   | `env`, `printenv`                                                | Exfiltrates secrets from environment. |
| Shell   | Any tool targeting `/etc/*`, `**/.ssh/*`, `**/.env`, `**/credentials*` | Protected paths. |
| SQL     | `DELETE`, `DROP *`, `TRUNCATE`, `GRANT`, `REVOKE`                | Destructive or privilege-changing. (`sqlgate` enforces `WHERE` on `DELETE`; the policy still hard-blocks the operation class to keep the agent inside an explicit migration workflow.) |
| HTTP    | `DELETE`                                                         | Almost never what you want from an automated PR-writer agent. |
| Global  | Any tool with capability tag `delete`                            | Backstop for tools that don't match a specific rule. |

## False positives the old pack had — and the fix

The original `pr-writer.yaml` had `default_decision: review` plus a small
allowlist, which meant:

- `git status`, `git log`, `git diff` → **prompted** (only `shell.git` was a
  blanket allow, but the read-only subcommands weren't called out, so it
  was unclear and brittle). **Fix:** explicit `allow` rules for every
  read-only `git` subcommand, then a final `shell.git → allow` fallback
  with `git push*` carved out to `review` and `--force` carved out to `block`.
- `pytest`, `go test`, `npm test` → only `pytest` was listed; others fell
  through to `review`. **Fix:** explicit allow for `pytest`, `go`, `cargo
  test/build/check`, `npm test`, `npm run build`, `make test/build/lint`.
- `ls`, `cat README.md` → `ls` was allowed but `cat` had block-rules
  before allow-rules, and any unusual filename could trip the wrong rule.
  **Fix:** secrets first (block), then a final `cat → allow`.
- Creating a branch → was already allow; kept that way and added
  `create_commit` and `create_or_update_file` for the same reason.
- Opening a PR on a feature branch → was already review; made it
  explicit per branch prefix (`agent/*`, `feat/*`, `fix/*`, `bugfix/*`)
  with a generic fallback so the same decision applies regardless of
  branch naming convention.

## Known limitations

1. **Glob targets depend on `policygate` glob support.** The patterns like
   `**/.ssh/*` and `agent/*` assume the policy engine matches them as
   shell globs. If your matcher is literal-only, replace with the
   appropriate prefix string.
2. **`shell.git` subcommand matching is target-string based.** An agent
   that runs `git --no-pager push` will skip the `git push*` rule. If you
   care about that, add the `--no-pager` variants or move the push check
   into a wrapper.
3. **No environment awareness.** The pack doesn't know `prod` from `dev`.
   For prod-aware policy, layer on `infra-review.yaml` and gate by which
   credentials are mounted.
4. **`DELETE` is blocked, not reviewed.** This is intentional — agents
   that need to delete rows should go through a reviewed migration. If
   your team genuinely runs ad-hoc `DELETE … WHERE`, downgrade
   `sql.delete` to `review` and rely on `sqlgate` to enforce the
   `WHERE` clause.
5. **HTTP rules don't distinguish endpoints.** All `POST`s look alike. If
   you want "allow `POST` to GitHub API but review everything else", add
   per-host rules above the generic ones.

## Customizing for your team

- **Stricter team?** Change `default_decision` to `block` and explicitly
  allow each tool you want. You'll trade convenience for an audit trail.
- **Looser team / trusted senior dev with an agent?** Promote
  `git push*` from `review` to `allow` for non-`main` targets, and
  promote `create_pull_request` to `allow`. Keep the destructive blocks.
- **Different branch conventions?** Edit the `target:` lines on the
  `create_pull_request` rules. The branch-agnostic fallback at the end
  ensures you don't get locked out if you forget one.
- **Monorepo with a `prod/` path?** Add a `shell.* target: "prod/*"
  decision: review` rule near the top.
- **CI account vs human account?** Run two policy packs: this one for
  the human, and a stricter variant (no `git push`, no PR creation) for
  the bot.

The fewer rules you can get away with, the better. Resist the urge to
add a rule for every edge case — every rule is one more thing that can
fire on innocent work and erode trust in the gate.
