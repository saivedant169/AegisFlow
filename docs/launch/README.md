# AegisFlow v0.9.0 launch bundle

Drafts and verified assets for v0.9.0 release. Review platform rules and final links before posting.

## One-line description

> AegisFlow is a local-first policy gateway for coding agents. It allows, reviews, or blocks routed MCP actions and verifies signed session evidence. Standalone execution gates remain experimental; runtime upstream credential issuance is disabled.

## Proof workflow

Recorded proof sends real requests through local MCP gateway:

1. Repository list is allowed.
2. Repository deletion is blocked.
3. Pull request creation waits for human review.
4. Exact approved retry is allowed.
5. Signed evidence chain verifies.

Mock upstream avoids real GitHub changes and credentials. See [governed pull request proof](../PR_WRITER.md).

## Drafts

| Platform | File | Focus |
|---|---|---|
| Show HN | [posts/hackernews-show-hn.md](posts/hackernews-show-hn.md) | Mechanism, proof, limits |
| Reddit | [posts/reddit.md](posts/reddit.md) | Operator problem and feedback request |
| Dev.to | [posts/devto-blog.md](posts/devto-blog.md) | Technical walkthrough |
| X | [posts/x-thread.md](posts/x-thread.md) | Short proof thread |
| LinkedIn | [posts/linkedin.md](posts/linkedin.md) | Security and platform teams |
| GitHub Discussions | [posts/github-discussions-announcement.md](posts/github-discussions-announcement.md) | Release announcement |

## Assets

- [15.23-second proof GIF](../assets/hero-pr-writer.gif)
- [blocked action](../assets/shot-blocked-action.png)
- [approval queue](../assets/shot-approval-queue.png)
- [evidence verification](../assets/shot-evidence-verification.png)
- [benchmark card](../assets/benchmark-card.png)
- [social preview](../assets/social-preview.png)
- [capture notes](asset-shot-list.md)

## Facts safe to cite

- v0.9.0, Apache-2.0, Go single binary.
- Real local proof covers allow, block, review, approved retry, and evidence verification.
- Release includes SHA-256 checksums, Sigstore bundle, SPDX SBOM, and GitHub build provenance.
- Apple M1 zero-latency mock benchmark: 55,327 req/s, 0.6 ms p50, 3.6 ms p99, 0 errors across 30,000 requests.
- Apple M1 25 ms mock provider benchmark with cache disabled: 624.57 req/s, 28.0 ms p50, 35.0 ms p99, 0.00% errors.

Do not describe microbenchmarks as external provider throughput. Do not claim local proof mints real GitHub credentials. Do not imply AegisFlow intercepts calls that bypass configured gateway.

## Posting order

1. Publish v0.9.0 release and Pages site.
2. Upload social preview and confirm release assets.
3. Post GitHub Discussions announcement.
4. Publish technical walkthrough on Dev.to.
5. Submit Show HN after links and screenshots resolve publicly.
6. Adapt shorter copy for Reddit, LinkedIn, and X.
