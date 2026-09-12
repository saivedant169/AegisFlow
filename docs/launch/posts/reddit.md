# I built a local policy gateway for coding agents and MCP tools

> Archived launch draft for v0.9.0. Do not publish as the development candidate announcement. Runtime upstream credential issuance is disabled; verify release artifacts and version-specific claims before reuse.


Coding agents can read repositories, run commands, call GitHub, and touch databases. I wanted policy decision before tool runs, not another log after damage.

Project is AegisFlow: Apache-2.0, Go, local-first. It sits between client and configured MCP or model endpoint. Every routed action gets normalized with actor, task, tool, target, arguments, and requested capability. Policy returns allow, review, or block.

v0.9.0 proof uses real local services against mock GitHub upstream:

- repository list reaches upstream
- repository deletion returns blocked error
- pull request waits in approval queue
- approved exact retry reaches upstream
- signed evidence chain verifies

GIF and commands: https://saivedant169.github.io/AegisFlow/PR_WRITER/

Release also adds tool-call handling across supported model adapters, per-session signed evidence, scoped GitHub App and AWS STS requests, checksum verification, SBOM, Sigstore bundle, and build provenance.

Boundary matters: AegisFlow cannot stop calls that bypass it. It does not sandbox editor process. Built-in file and terminal actions need explicit routing through MCP or another supported path.

Repo: https://github.com/saivedant169/AegisFlow

Question for people operating coding agents: which actions do you route to human approval today, and what context would reviewer need before approving one?
