[![CI](https://github.com/AndriyKalashnykov/dapr-go-crud-app/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/AndriyKalashnykov/dapr-go-crud-app/actions/workflows/ci.yml)
[![Hits](https://hits.sh/github.com/AndriyKalashnykov/dapr-go-crud-app.svg?view=today-total&style=plastic)](https://hits.sh/github.com/AndriyKalashnykov/dapr-go-crud-app/)
[![Renovate enabled](https://img.shields.io/badge/renovate-enabled-brightgreen.svg)](https://app.renovatebot.com/dashboard#github/AndriyKalashnykov/dapr-go-crud-app)

# Dapr Go CRUD App

A set of Go microservices demonstrating Dapr (Distributed Application Runtime) features including CRUD operations, pub/sub messaging, service invocation, and timeline tracking. Deploys to Kubernetes using ko for container image building.

## Quick Start

```bash
make deps      # install/check required tools
make build     # build all binaries
make test      # run tests
make ci        # full CI pipeline (format, lint, test, build)
```

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| [gvm](https://github.com/moovweb/gvm) | latest | Go Version Manager (auto-installs required Go versions) |
| [GNU Make](https://www.gnu.org/software/make/) | 3.81+ | Build orchestration |
| [Docker](https://www.docker.com/) | latest | Container runtime |
| [kubectl](https://kubernetes.io/docs/tasks/tools/) | latest | Kubernetes CLI |
| [ko](https://ko.build/) | 0.18+ | Go container image builder |
| [golangci-lint](https://golangci-lint.run/) | 2.1+ | Go linters aggregator (auto-installed by `make deps`) |
| [Dapr CLI](https://docs.dapr.io/getting-started/install-dapr-cli/) | latest | Local Dapr development (optional) |
| [Helm](https://helm.sh/) | latest | Redis deployment to Kubernetes (optional) |
| [act](https://github.com/nektos/act) | latest | Run GitHub Actions locally (optional) |

Install all required dependencies:

```bash
make deps
```

## Available Make Targets

Run `make help` to see all available targets.

### Build & Run

| Target | Description |
|--------|-------------|
| `make build` | Build all binaries |
| `make test` | Run tests with race detection |
| `make lint` | Run golangci-lint (includes gocritic via .golangci.yml) |
| `make format` | Format Go source files |
| `make clean` | Remove build artifacts |
| `make run` | Run the main app locally |
| `make update` | Update Go dependencies |

### Dapr & Infrastructure

| Target | Description |
|--------|-------------|
| `make dapr-run` | Run app with Dapr sidecar |
| `make mongo-run` | Run MongoDB in Docker |
| `make redis-deploy` | Deploy Redis with Helm (standalone) |
| `make redis-deploy-replicated` | Deploy Redis with Helm (replication) |
| `make zipkin-setup` | Deploy Zipkin and expose via skind |
| `make zipkin-deploy` | Deploy Zipkin to Kubernetes |

### Kubernetes Deployment

| Target | Description |
|--------|-------------|
| `make deploy` | Deploy full stack to Kubernetes |
| `make apply` | Apply Dapr config and deployments |
| `make push` | Publish all images with ko |
| `make rollout` | Restart app pods |
| `make app-logs` | Show crud-app container logs |

### CI

| Target | Description |
|--------|-------------|
| `make ci` | Run full CI pipeline locally |
| `make ci-run` | Run GitHub Actions workflow locally using [act](https://github.com/nektos/act) |

### Utilities

| Target | Description |
|--------|-------------|
| `make help` | List available tasks |
| `make deps` | Check and install required dependencies (uses gvm for Go) |
| `make deps-check` | Show required Go versions and gvm status |
| `make deps-act` | Install act for local CI |
| `make deps-prune` | Remove unused Go dependencies |
| `make deps-prune-check` | Verify no prunable dependencies (CI gate) |
| `make release` | Create and push a new tag |
| `make renovate-validate` | Validate Renovate configuration |

## CI/CD

GitHub Actions runs on every push to `main`, tags `v*`, and pull requests.

| Job | Triggers | Steps |
|-----|----------|-------|
| **static-check** | push (main), PR, tags | Lint |
| **build** | after static-check | Build, upload artifacts |
| **test** | after static-check | Test |
| **cleanup** | weekly (Sunday) + manual | Delete old workflow runs (retain 7 days, keep 5 minimum) |

Concurrency is set with `cancel-in-progress: true`. Permissions are minimal (`contents: read`).

[Renovate](https://docs.renovatebot.com/) keeps dependencies up to date with platform automerge enabled.

## References

- [Original crud app](https://github.com/famarting/crud-app)
