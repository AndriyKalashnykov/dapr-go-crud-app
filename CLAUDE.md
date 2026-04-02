# CLAUDE.md

## Project Overview

Go microservices project demonstrating Dapr (Distributed Application Runtime) features including CRUD operations, pub/sub messaging, service invocation, and timeline tracking. Deploys to Kubernetes using ko for image building.

## Tech Stack

- **Language**: Go 1.26.1
- **Framework**: Gin (HTTP), Dapr Go SDK
- **Infrastructure**: Kubernetes, Redis (state store), Dapr sidecars
- **Build**: ko (container images), Make (task runner)
- **CI**: GitHub Actions

## Project Structure

```
cmd/           - Application entry points (app, consumer, datagen, dummy, errorgen, publisher, service-a/b/c, timeline)
pkg/           - Shared packages
deploy/        - Kubernetes manifests
.dapr/         - Dapr configuration and components
.bin/          - Build output (gitignored)
.github/       - CI workflows
```

## Key Variables

| Variable | Value | Purpose |
|----------|-------|---------|
| `GOLANGCI_VERSION` | `2.11.4` | golangci-lint version |
| `KO_VERSION` | `0.18.1` | ko image builder version |
| `ACT_VERSION` | `0.2.87` | act (local CI) version |
| `NVM_VERSION` | `0.40.4` | nvm version for Renovate |
| `NODE_VERSION` | `24` | Node.js version for nvm |
| `GVM_SHA` | `dd6525...` | gvm commit SHA (v1.0.22) |
| `APP_NAMESPACE` | `crud-app` | Kubernetes namespace |

## Build & Test

```bash
make help             # List all available targets
make deps             # Check and install required dependencies (uses gvm for Go)
make deps-check       # Show required Go versions and gvm status
make build            # Build all binaries
make test             # Run tests with race detection
make lint             # Run golangci-lint (includes gocritic via .golangci.yml)
make format           # Format Go source files
make ci               # Full CI pipeline (format, lint, test, build)
make clean            # Remove build artifacts
make deps-prune       # Remove unused Go dependencies
make deps-prune-check # Verify no prunable dependencies (CI gate)
```

## Run Locally

```bash
make run          # Run main app (requires Dapr)
make dapr-run     # Run with Dapr sidecar
make mongo-run    # Start MongoDB in Docker
```

## Deploy

```bash
make deploy       # Full stack deploy to Kubernetes
make push         # Publish all images with ko
make rollout      # Restart app pods
make apply        # Apply Dapr config and deployments
```

## CI/CD

GitHub Actions workflow (`.github/workflows/ci.yml`) runs on every push to `main`, tags `v*`, pull requests, and via `workflow_call` (reusable).

Jobs (separate, with dependency edges):
1. **static-check** — Checkout, Setup Go, Lint (`make lint`)
2. **build** (needs: static-check) — Checkout, Setup Go, Build (`make build`), upload artifacts
3. **test** (needs: static-check) — Checkout, Setup Go, Test (`make test`)

`build` and `test` run in parallel after `static-check` passes. Concurrency is set with `cancel-in-progress: true`. Permissions: `contents: read` (minimal).

A separate cleanup workflow (`.github/workflows/cleanup-runs.yml`) removes old workflow runs weekly (Sunday cron) and supports manual trigger via `workflow_dispatch`. Permissions: `actions: write`.

Run CI locally: `make ci` (local pipeline) or `make ci-run` (GitHub Actions via act).

## Dependencies

- gvm (Go Version Manager) — manages Go versions locally; `make deps` auto-installs gvm and required versions
- docker (check with `make deps`)
- golangci-lint, ko (auto-installed by `make deps`)
- kubectl for Kubernetes deployment
- Helm for Redis deployment
- Dapr CLI for local development

In CI, `actions/setup-go` provides Go — gvm is not needed.

## Skills

Use the following skills when working on related files:

| File(s) | Skill |
|---------|-------|
| `Makefile` | `/makefile` |
| `renovate.json` | `/renovate` |
| `README.md` | `/readme` |
| `.github/workflows/*.yml` | `/ci-workflow` |

When spawning subagents, always pass conventions from the respective skill into the agent's prompt.
