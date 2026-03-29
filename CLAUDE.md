# CLAUDE.md

## Project Overview

Go microservices project demonstrating Dapr (Distributed Application Runtime) features including CRUD operations, pub/sub messaging, service invocation, and timeline tracking. Deploys to Kubernetes using ko for image building.

## Tech Stack

- **Language**: Go 1.26
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
| `GOLANGCI_VERSION` | `2.1.6` | golangci-lint version |
| `KO_VERSION` | `0.18.0` | ko image builder version |
| `ACT_VERSION` | `0.2.86` | act (local CI) version |
| `NVM_VERSION` | `0.40.4` | nvm version for Renovate |
| `APP_NAMESPACE` | `crud-app` | Kubernetes namespace |

## Build & Test

```bash
make help      # List all available targets
make deps      # Check and install required dependencies
make build     # Build all binaries
make test      # Run tests
make lint      # Run golangci-lint
make ci        # Full CI pipeline (deps, lint, test, build)
make clean     # Remove build artifacts
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

GitHub Actions workflow (`.github/workflows/ci.yml`) runs on every push to `main`, tags `v*`, and pull requests:
1. Checkout with full history
2. Setup Go from `go.mod`
3. Build (`make build`)
4. Lint (`make lint`)
5. Test (`make test`)

A separate cleanup workflow (`.github/workflows/cleanup-runs.yml`) removes old workflow runs weekly.

Run CI locally: `make ci` (local pipeline) or `make ci-run` (GitHub Actions via act).

## Dependencies

- go, docker (check with `make deps`)
- golangci-lint, ko (auto-installed by `make deps`)
- kubectl for Kubernetes deployment
- Helm for Redis deployment
- Dapr CLI for local development

## Skills

Use the following skills when working on related files:

| File(s) | Skill |
|---------|-------|
| `Makefile` | `/makefile` |
| `renovate.json` | `/renovate` |
| `README.md` | `/readme` |
| `.github/workflows/*.yml` | `/ci-workflow` |

When spawning subagents, always pass conventions from the respective skill into the agent's prompt.
