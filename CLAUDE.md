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

## Build & Test

```bash
make help      # List all available targets
make build     # Build all binaries
make test      # Run tests
make lint      # Run golangci-lint
make ci        # Full CI pipeline (lint, test, build)
make clean     # Remove build artifacts
make deps      # Check required dependencies
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

## Dependencies

- go, docker, kubectl, ko (check with `make deps`)
- golangci-lint for linting
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
