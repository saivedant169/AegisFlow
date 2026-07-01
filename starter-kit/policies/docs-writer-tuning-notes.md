# Docs Writer Policy Pack - Tuning Notes

## Philosophy: understand code, write docs

`docs-writer.yaml` is for agents that improve documentation but should not
modify source code. The agent can inspect the whole repository so it can write
accurate docs, but its write surface stays limited to Markdown and common docs
locations.

Use this pack for README rewrites, API docs, changelog drafts, contributor
guides, troubleshooting docs, and docs-site updates.

Do not use this pack for feature work, migrations, infra changes, or production
operations. Use `pr-writer.yaml`, `infra-review.yaml`, or `sql-explorer.yaml`
instead.

## What's allowed

| Area | Allowed | Why |
|------|---------|-----|
| GitHub | `list_*`, `get_*`, `search_*` | The agent needs context before editing docs. |
| GitHub | `create_branch`, `create_commit` | Docs work still needs normal branch/commit flow. |
| Shell | `ls`, `pwd`, `grep`, `find`, `head`, `tail`, `wc`, `diff`, `less`, safe `cat` | Reading and comparing files is core docs work. |
| Shell | read-only `git` commands, `git checkout`, `git switch`, `git fetch`, `git add`, `git commit`, `git stash` | Local git workflow for docs branches. |
| Shell | test/build/lint commands | Docs changes often need link, formatting, or site-build checks. |
| SQL | `SELECT` | Read-only inspection is okay when documenting data behavior. |
| HTTP | `GET`, `HEAD` | Fetching public docs or checking links is okay. |

## What's reviewed

| Area | Reviewed | Why |
|------|----------|-----|
| GitHub | `create_pull_request` | The PR is the human review checkpoint. |
| GitHub | docs-file `create_or_update_file` | Direct GitHub writes should stay visible to a reviewer. |
| Shell | `git push`, `git reset --hard` | Leaves local-only safety or changes local state sharply. |
| Shell | package install and generic build tools | Can run arbitrary project scripts. |
| Shell | `docker`, `kubectl`, `terraform` | Usually outside docs-only scope. |
| SQL | `INSERT`, `UPDATE` | Mutates data, even if a docs task asks for examples. |
| HTTP | `POST`, `PUT`, `PATCH` | Mutates remote systems. |

## What's blocked

| Area | Blocked | Why |
|------|---------|-----|
| GitHub | repo deletion, force push, deleting `main`/`master`, inviting collaborators | Administrative or destructive. |
| GitHub | direct writes to common source-code extensions | Docs agents should open docs PRs, not patch product code. |
| Shell | direct edits to common source-code extensions | Keeps the agent inside docs-only work. |
| Shell | destructive commands and protected paths | Same safety baseline as `pr-writer`. |
| SQL | `DELETE`, `DROP`, `TRUNCATE`, `GRANT`, `REVOKE` | Never part of documentation authoring. |
| HTTP | `DELETE` | Destructive remote mutation. |
| Global | delete capability | Backstop for unclassified tools. |

## Why source-code writes are blocked

A docs agent often needs to read implementation files to understand behavior.
That is different from being allowed to modify them. If the agent discovers a
real code bug while documenting, it should mention it in the PR or open a
follow-up issue, not silently mix code changes into a docs PR.

## Customizing for your team

- Add extra docs paths if your repo uses `handbook/`, `website/docs/`, or
  `content/`.
- Downgrade docs-file GitHub writes from `review` to `allow` if your team wants
  fully automated docs branches.
- If generated docs live beside source files, add the exact generated path
  before the broad source-code blocks.
- If your docs build requires a specific command, add an explicit `allow` rule
  above the generic package-manager review rules.
