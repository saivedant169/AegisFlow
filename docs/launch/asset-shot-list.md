# Launch asset shot-list

The text posts are ready in this folder. These visual assets still need to be
captured by hand (they require a screen recorder / image tool). Each entry
lists exactly what to capture so the bundle is consistent.

## 1. Hero GIF — the governed PR-writer flow (the one asset that matters most)

Record the `install-pr-writer.sh` demo driving a coding agent end to end:

1. `./starter-kit/install-pr-writer.sh` finishing ("ready in ~10s")
2. agent reads the repo (allow)
3. agent runs tests (allow)
4. agent attempts `rm -rf` → **blocked** (show the -32001 error)
5. agent opens a PR → **review required** (show the approval queue)
6. `aegisctl approve …` → scoped credential minted
7. `aegisctl evidence verify` → `valid: true`

- Length: 15–25 s, loop-friendly. Trim dead air.
- Save as: `docs/assets/hero-pr-writer.gif`, then uncomment the hero slot in `README.md`.
- Tools: `asciinema` + `agg`, or any screen recorder → `ffmpeg`/`gifski`.

## 2. Three screenshots

- `docs/assets/shot-block.png` — terminal showing a destructive action blocked (`shell.rm` → `-32001`).
- `docs/assets/shot-approval.png` — the admin approval queue holding a `github.create_pull_request` with the diff title + justification.
- `docs/assets/shot-evidence.png` — `aegisctl evidence verify` output ending in `valid: true, total_entries: N`.

## 3. Benchmark card image

- Render `docs/launch/benchmark-card.md` (the ASCII box) as a PNG, or screenshot it.
- Save as: `docs/assets/benchmark-card.png`.

## 4. Social preview (repo Settings → Social preview)

- 1280×640. Title "AegisFlow", the one-line pitch, the allow/review/block words.
- Upload via GitHub repo Settings → Options → Social preview.

## Where each asset is used

| Asset | Used in |
|-------|---------|
| hero GIF | README top, dev.to post, X tweet 1 |
| shot-block / shot-approval / shot-evidence | dev.to, Reddit, LinkedIn |
| benchmark card | HN comment, X thread, LinkedIn |
| social preview | every shared link |
