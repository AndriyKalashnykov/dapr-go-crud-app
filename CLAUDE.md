# CLAUDE.md

## Project Overview

10-binary Go microservice topology demonstrating Dapr building blocks (state store, pub/sub, service invocation, resiliency) on Kubernetes. Container images built with `ko` (no Dockerfile). Toolchain managed end-to-end by `mise` (`.mise.toml`).

## Tech Stack

- **Language**: Go 1.26.2 (pinned in `.mise.toml`)
- **HTTP**: Gin
- **Distributed runtime**: Dapr Go SDK
- **Storage**: Redis (Dapr state store + pub/sub broker), MongoDB 8.0 (alternative crud-app backend)
- **Container build**: ko 0.18 (publishes multi-arch images directly from Go source — no Dockerfile)
- **Image registry**: GHCR `ghcr.io/andriykalashnykov/dapr-go-crud-app/<binary>` (repo-namespace path; `GITHUB_TOKEN` cannot publish to user-namespace)
- **Image supply-chain**: Trivy image scan (HIGH/CRITICAL blocking) → smoke test → cosign keyless OIDC signing by digest. Buildkit attestations OFF (Pattern A) so the GHCR "OS / Arch" tab renders
- **Orchestration**: Kubernetes (Dapr injector + sidecars)
- **Toolchain manager**: mise — single source of truth for Go, Node, golangci-lint, ko, act, gosec, gitleaks, trivy, actionlint, shellcheck, govulncheck
- **CI**: GitHub Actions (composite `make static-check` gate, `dorny/paths-filter` change detector, `ci-pass` aggregator)
- **Static analysis**: golangci-lint (gocritic enabled), gosec, govulncheck, gitleaks, trivy (fs + config), actionlint, shellcheck, mermaid-cli

## Project Structure

```
cmd/           - 10 entry points (one subdir each): app, consumer, datagen, dummy, errorgen, publisher, service-a/b/c, timeline
pkg/           - Shared packages: server, storage, timeline, todos
deploy/        - Kubernetes manifests (one per app)
.dapr/         - Dapr configuration + components (pubsub, state, resiliency, subscriptions)
.bin/          - Build output (gitignored)
.github/       - CI workflows
scripts/       - Helper scripts (mermaid-lint extractor)
.mise.toml     - Toolchain pins (go, node, every static-analysis binary)
```

## Key Config

| Variable | Value | Purpose |
|----------|-------|---------|
| `KO_DOCKER_REPO` | `ghcr.io/andriykalashnykov/dapr-go-crud-app` (override via env) | Default ko publish target — repo-namespace path so `GITHUB_TOKEN` can publish |
| `KO_PLATFORMS` | `linux/amd64,linux/arm64` | Multi-arch publish target (override for faster single-arch dev builds) |
| `APP_NAMESPACE` | `crud-app` | Kubernetes namespace |
| `MONGO_VERSION` | `8.0` | Renovate-tracked Docker image |
| `ZIPKIN_VERSION` | `3.6` | Renovate-tracked Docker image |
| `ACT_UBUNTU_VERSION` | `act-24.04` | Renovate-tracked `catthehacker/ubuntu` image used by `make ci-run` |
| `MERMAID_CLI_VERSION` | `11.4.2` | Renovate-tracked mermaid-cli Docker image |
| `RETAIN_DAYS` / `KEEP_MINIMUM` | 7 / 5 | `cleanup-runs.yml` retention knobs |

Tool versions live in `.mise.toml` and are tracked by Renovate's native `mise` manager. Inline `# renovate:` comments in the Makefile cover the Docker-image constants above via the generic `custom.regex` manager.

## Build & Test

```bash
make help             # List all targets
make deps             # Install mise + every pinned tool from .mise.toml
make build            # Compile all 10 binaries
make test             # Unit tests: go test -race ./... (seconds, no infra)
make integration-test # Testcontainers: go test -race -tags=integration -v ./... (Mongo via Testcontainers; needs Docker)
make e2e              # Full chain: kind-up + dapr-install + e2e-redis-deploy + e2e-apply + e2e-smoke (minutes, needs Docker)
make e2e-smoke        # Smoke-only against an already-deployed cluster (CI-style)
make static-check     # Composite gate: format + lint + lint-ci + shellcheck + sec + vulncheck + secrets + trivy-fs + trivy-config + mermaid-lint
make ci               # Full local pipeline: deps + static-check + test + integration-test + build
make ci-run           # Run GitHub Actions workflow locally via act
make clean            # Remove .bin and coverage output
make deps-prune       # go mod tidy
make deps-prune-check # Verify go.mod/go.sum tidy (CI gate)
```

Three-layer test pyramid:

| Layer | Target | Runtime | Infra |
|-------|--------|---------|-------|
| Unit | `make test` | seconds | none |
| Integration | `make integration-test` | tens of seconds | Docker (Testcontainers) |
| E2E | `make e2e` / `make e2e-smoke` | minutes | KinD + Dapr Helm + upstream redis:8 |

## Run Locally

```bash
make run          # Run crud-app (requires Dapr sidecar)
make dapr-run     # Run crud-app under `dapr run` (depends on build)
make mongo-run    # Run MongoDB in Docker (alternative storage backend)
```

## Deploy

```bash
make redis-deploy   # Apply deploy/redis.yaml (upstream redis:8-alpine, standalone)
make deploy         # Apply Dapr config + components + manifests (no image rebuild)
make deploy-full    # ko publish + redis + apply (one-shot)
make rollout        # Restart crud-app + timeline-app pods
make app-logs       # Tail crud-app logs
```

## CI/CD

Two workflows under `.github/workflows/`:

### `ci.yml`

Triggers: push to `main`, `v*` tags, pull requests, and `workflow_call`.

Jobs (with explicit dependency edges):

1. **`changes`** — `dorny/paths-filter` (SHA-pinned). Outputs `code` = true on any non-Markdown change. Downstream jobs are skipped on doc-only updates.
2. **`static-check`** — `needs: [changes]`, gated by `code`. Runs `make static-check`.
3. **`build`** — `needs: [changes, static-check]`. Runs `make build`, uploads `.bin/` artifacts.
4. **`test`** — `needs: [changes, static-check]`. Runs `make test` (unit).
5. **`integration-test`** — `needs: [changes, static-check]`. Runs `make integration-test` (Testcontainers Mongo + httptest).
6. **`e2e`** — `needs: [changes, build]`. Provisions KinD via `helm/kind-action`, installs Dapr via Helm, applies `deploy/redis.yaml` (upstream `redis:8-alpine` standalone — no Helm chart) plus the app manifests, runs `make e2e-smoke`. Diagnostic dump on failure.
7. **`docker`** — `needs: [static-check, build, test, integration-test, e2e]`, `if: startsWith(github.ref, 'refs/tags/')`, `strategy.matrix.binary` across all 10 cmds. Per /harden-image-pipeline Pattern A: ko build local → Trivy image scan → smoke test → ko publish multi-arch to GHCR → cosign keyless OIDC signing by digest. `provenance: false`/`sbom: false` (default — keeps the image index free of `unknown/unknown` entries so GHCR "OS / Arch" tab renders). Permissions: `contents: read`, `packages: write`, `id-token: write` (all job-scoped).
8. **`ci-pass`** — `needs: [changes, static-check, build, test, integration-test, e2e, docker]`, `if: always()`. Aggregator that fails if any required job failed/cancelled. The `docker` job is `skipped` on non-tag pushes — handled correctly. Single status check to require in branch protection.

Concurrency: `cancel-in-progress: true`. Permissions: `contents: read` (jobwise). Setup is via `jdx/mise-action` (no `actions/setup-go` — mise provides Go).

### `cleanup-runs.yml`

Weekly (Sunday midnight UTC) plus `workflow_dispatch`. Calls `make cleanup-runs` so the same logic runs locally with `RETAIN_DAYS` / `KEEP_MINIMUM` overrides. Permissions: `actions: write`.

## Dependencies

All toolchain (Go, Node, golangci-lint, ko, act, gosec, gitleaks, trivy, actionlint, shellcheck, govulncheck) is installed by `mise install` from `.mise.toml` — `make deps` does this automatically. The only host requirements are GNU Make, Git, and Docker.

In CI, `jdx/mise-action` reads `.mise.toml` and installs the same tool set — no separate `actions/setup-go` step is needed.

## Renovate

`renovate.json` enables five managers:

- `gomod` — go.mod
- `github-actions` — workflow `uses:` refs
- `kubernetes` — `image:` fields in `deploy/*.yaml` (scoped via `kubernetes.managerFilePatterns`)
- `mise` — `.mise.toml` (`aqua:` and `go:` backends are recognised; queried via GitHub tags)
- `custom.regex` — Makefile constants annotated with `# renovate:` comments

Toolchain bumps (Makefile + `.mise.toml`) are grouped into a single PR; Dapr SDK bumps grouped separately. Vulnerability fixes fast-tracked (`minimumReleaseAge: "0 days"`); major updates wait 3 days. `automergeType: "pr"` (compatible with branch-protection required-status-checks).

## Skills

Use the following skills when working on related files:

| File(s) | Skill |
|---------|-------|
| `Makefile` | `/makefile` |
| `.mise.toml` | `/makefile` |
| `renovate.json` | `/renovate` |
| `README.md` | `/readme` |
| `.github/workflows/*.{yml,yaml}` | `/ci-workflow` |

When spawning subagents, always pass conventions from the respective skill into the agent's prompt.
