SHELL := /bin/bash

.DEFAULT_GOAL := help

# Put mise shims first so recipe sub-shells see mise-managed binaries.
export PATH := $(HOME)/.local/share/mise/shims:$(HOME)/.local/bin:$(PATH)

APP_NAME       := dapr-go-crud-app
CURRENTTAG     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")

# === Image versions (Renovate-tracked) ===

# renovate: datasource=docker depName=mongo
MONGO_VERSION := 8.0

# renovate: datasource=docker depName=openzipkin/zipkin
ZIPKIN_VERSION := 3.6

# renovate: datasource=docker depName=catthehacker/ubuntu versioning=loose
ACT_UBUNTU_VERSION := act-24.04

# renovate: datasource=docker depName=minlag/mermaid-cli
MERMAID_CLI_VERSION := 11.4.2

# === Registry (overridable from env) ===
# Default to GHCR repo-namespace path so `GITHUB_TOKEN` can publish
# (the user-namespace path `ghcr.io/andriykalashnykov/<pkg>` requires a
# PAT — see /harden-image-pipeline §"GHCR namespace rule").
export KO_DOCKER_REPO ?= ghcr.io/andriykalashnykov/dapr-go-crud-app

# ko-published image short-name strategy: --base-import-paths uses just
# the leaf directory under cmd/ (e.g. ./cmd/consumer → "consumer"). All
# 10 binaries follow the same `./cmd/<name>` shape, so one BINARY_DIRS
# list covers everything.
APP_NAMESPACE ?= crud-app

BINARY_DIRS := app consumer datagen dummy errorgen publisher service-a service-b service-c timeline

# Multi-arch publish target. Override locally with `make push KO_PLATFORMS=linux/amd64`
# for faster single-arch dev builds.
KO_PLATFORMS ?= linux/amd64,linux/arm64

# Cosign is pinned in `.mise.toml` (`aqua:sigstore/cosign`) and the
# CI workflow uses `sigstore/cosign-installer` (SHA-pinned). No Makefile
# constant is needed — the previous `COSIGN_VERSION` constant was unused
# and duplicated the .mise.toml pin (per /makefile skill mise-migration
# rule: drop Makefile constants whose value lives in .mise.toml).

# Cleanup workflow knobs (override at invocation: make cleanup-runs RETAIN_DAYS=14)
RETAIN_DAYS  ?= 7
KEEP_MINIMUM ?= 5
GH_REPO      ?= $(shell gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null)

# === KinD / Dapr e2e ===
KIND_CLUSTER_NAME ?= dapr-go-crud-app
# renovate: datasource=helm depName=dapr registryUrl=https://dapr.github.io/helm-charts
DAPR_HELM_VERSION ?= 1.17.1
# Context-pinned wrappers — keep e2e flow safely scoped to the kind cluster
# regardless of any other ~/.kube/config currently-context drift across
# parallel make invocations from sibling projects.
KUBECTL_E2E := kubectl --context=kind-$(KIND_CLUSTER_NAME)
HELM_E2E    := helm --kube-context=kind-$(KIND_CLUSTER_NAME)

#help: @ List available tasks
help:
	@echo "Usage: make COMMAND"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-30s\033[0m - %s\n", $$1, $$2}'

#deps: @ Install mise and project tools (.mise.toml)
deps:
	@command -v mise >/dev/null 2>&1 || { \
		echo "Installing mise (no root, ~/.local/bin)..."; \
		curl -fsSL https://mise.run | sh; \
	}
	@command -v docker >/dev/null 2>&1 || { echo "Error: Docker is required. Install from https://www.docker.com/"; exit 1; }
	@mise install --yes

#test: @ Run unit tests with race detection
test: deps
	@go test -race ./...

#integration-test: @ Run integration tests (Testcontainers; requires Docker)
integration-test: deps
	@go test -race -tags=integration -v ./...

#build: @ Build all binaries
build: deps
	@for d in $(BINARY_DIRS); do go build -o ./.bin/$$d ./cmd/$$d || exit 1; done

#clean: @ Remove build artifacts and coverage output
clean:
	@rm -rf ./.bin coverage.out

#lint: @ Run golangci-lint (gocritic enabled via .golangci.yml)
lint: deps
	@golangci-lint run ./...

#format: @ Format Go source files (mutates tree)
format: deps
	@gofmt -w .

#format-check: @ Verify Go sources are gofmt-clean (CI gate)
format-check: deps
	@unformatted=$$(gofmt -l . 2>/dev/null); \
	if [ -n "$$unformatted" ]; then \
		echo "ERROR: the following files are not gofmt-clean. Run 'make format':"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

#sec: @ Run gosec (HIGH/CRITICAL gate)
sec: deps
	@gosec -quiet -severity high -confidence high ./...

#vulncheck: @ Run govulncheck against project + std library
vulncheck: deps
	@govulncheck ./...

#secrets: @ Scan working tree for committed secrets
secrets: deps
	@gitleaks detect --no-banner --redact --exit-code 1

#trivy-fs: @ Trivy filesystem scan (vuln, secret, misconfig)
trivy-fs: deps
	@trivy fs --quiet --exit-code 1 --scanners vuln,secret,misconfig --severity HIGH,CRITICAL --skip-dirs .bin,vendor,node_modules .

#trivy-config: @ Trivy IaC scan over k8s + Dapr manifests
trivy-config: deps
	@trivy config --quiet --exit-code 1 --severity HIGH,CRITICAL deploy
	@trivy config --quiet --exit-code 1 --severity HIGH,CRITICAL .dapr

#lint-ci: @ Lint GitHub Actions workflows
lint-ci: deps
	@actionlint -color

#shellcheck: @ Lint shell scripts under scripts/
shellcheck: deps
	@if [ -d scripts ]; then \
		find scripts -type f -name '*.sh' -print0 | xargs -0 -r shellcheck; \
	fi

#mermaid-lint: @ Validate ```mermaid blocks in markdown via official CLI
mermaid-lint:
	@MERMAID_CLI_VERSION=$(MERMAID_CLI_VERSION) bash scripts/mermaid-lint.sh

#static-check: @ Composite quality gate (lint + ci + sec + vulncheck + secrets + trivy + mermaid)
static-check: format-check lint lint-ci shellcheck sec vulncheck secrets trivy-fs trivy-config mermaid-lint

#run: @ Run the main app locally (requires Dapr sidecar)
run: deps
	@go run ./cmd/app.go serve -connStr dapr

#update: @ Update Go dependencies (aggressive — major bumps allowed)
update: deps
	@go get -u ./...
	@go mod tidy

#update-minor: @ Update Go dependencies to latest patch only
update-minor: deps
	@go get -u=patch ./...
	@go mod tidy

#image-build: @ Build images locally (no push, single-arch, ko.local prefix)
image-build: deps
	@for d in $(BINARY_DIRS); do \
		KO_DOCKER_REPO=ko.local ko build --local --base-import-paths --tags=scan ./cmd/$$d || exit 1; \
	done

#image-scan: @ Trivy scan locally-built ko images (CRITICAL/HIGH blocking)
image-scan: image-build
	@for d in $(BINARY_DIRS); do \
		echo "==> trivy image ko.local/$$d:scan"; \
		trivy image --quiet --exit-code 1 --severity HIGH,CRITICAL --ignore-unfixed "ko.local/$$d:scan" || exit 1; \
	done

#image-smoke-test: @ Boot each ko-built binary briefly and verify it doesn't crash
image-smoke-test: image-build
	@refs=""; for d in $(BINARY_DIRS); do refs="$$refs ko.local/$$d:scan"; done; \
		bash scripts/image-smoke-test.sh $$refs

#push: @ Publish images with ko (multi-arch by default; gated on image-scan + image-smoke-test)
push: deps image-scan image-smoke-test
	@for d in $(BINARY_DIRS); do \
		ko publish --base-import-paths --platform=$(KO_PLATFORMS) ./cmd/$$d || exit 1; \
	done

#image-sign: @ Cosign keyless-sign every image:tag pushed by `make push` (operator's gh identity)
image-sign: deps
	@for d in $(BINARY_DIRS); do \
		ref="$(KO_DOCKER_REPO)/$$d:latest"; \
		digest=$$(crane digest "$$ref" 2>/dev/null) || { echo "skip $$d (no published tag yet)"; continue; }; \
		echo "==> cosign sign $$ref@$$digest"; \
		cosign sign --yes "$$ref@$$digest" || exit 1; \
	done

#rollout: @ Restart app pods
rollout:
	@kubectl delete pod -l app=crud-app -n $(APP_NAMESPACE)
	@kubectl delete pod -l app=timeline-app -n $(APP_NAMESPACE)

#app-logs: @ Show crud-app container logs
app-logs:
	@kubectl logs -l app=crud-app -c crud-app -n $(APP_NAMESPACE)

#mongo-run: @ Run MongoDB in Docker
mongo-run:
	@docker run -it -p 27017:27017 mongo:$(MONGO_VERSION)

#dapr-run: @ Run app under Dapr sidecar (depends on build)
dapr-run: build
	@dapr run --app-id crud-app --app-port 8080 --dapr-http-port 3500 -- ./.bin/app serve -connStr dapr

#zipkin-deploy: @ Deploy Zipkin to the current namespace
zipkin-deploy:
	@kubectl create deployment zipkin --image openzipkin/zipkin:$(ZIPKIN_VERSION) -n $(APP_NAMESPACE)
	@kubectl expose deployment zipkin --type ClusterIP --port 9411 -n $(APP_NAMESPACE)

#redis-deploy: @ Deploy upstream redis (standalone) into the current namespace
redis-deploy:
	@kubectl create namespace $(APP_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@# Generate the redis-password Secret only if it doesn't already exist —
	@# regenerating on every run would break the running Redis (server boots
	@# with the old password; new clients try the new). Per /security:
	@# `--from-file=KEY=/dev/stdin` (stdin form) keeps the value out of argv.
	@if ! kubectl get secret redis -n $(APP_NAMESPACE) >/dev/null 2>&1; then \
		printf '%s' "$$(openssl rand -base64 16)" | \
			kubectl create secret generic redis -n $(APP_NAMESPACE) \
				--from-file=redis-password=/dev/stdin \
				--dry-run=client -o yaml | \
			kubectl apply -f -; \
	fi
	@kubectl apply -f deploy/redis.yaml -n $(APP_NAMESPACE)
	@kubectl rollout status deployment/redis -n $(APP_NAMESPACE) --timeout=120s

#apply: @ Apply Dapr config and deployments (no image push)
apply:
	@kubectl create namespace $(APP_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@kubectl apply -f .dapr/configuration.yaml -n $(APP_NAMESPACE)
	@kubectl apply -f .dapr/components -n $(APP_NAMESPACE)
	@kubectl apply -f deploy -n $(APP_NAMESPACE)

#deploy: @ Apply manifests without rebuilding images (fast iteration)
deploy: redis-deploy apply

#deploy-full: @ Build/push images then deploy full stack
deploy-full: push deploy

#release: @ Create and push a new tag
release:
	@bash -c 'read -p "New tag (current: $(CURRENTTAG)): " newtag; \
		echo "$$newtag" | grep -qE "^v[0-9]+\.[0-9]+\.[0-9]+$$" || { echo "Error: Tag must match vN.N.N"; exit 1; }; \
		read -p "Create and push $$newtag? [y/N] " ans; \
		[ "$${ans:-N}" = "y" ] || { echo "Aborted."; exit 1; }; \
		echo $$newtag > ./version.txt; \
		git add version.txt; \
		git commit -a -s -m "Cut $$newtag release"; \
		git tag $$newtag; \
		git push; \
		git push origin $$newtag; \
		echo "Done."'

#renovate-validate: @ Validate Renovate configuration
renovate-validate: deps
	@if [ -n "$$GH_ACCESS_TOKEN" ]; then \
		GITHUB_COM_TOKEN=$$GH_ACCESS_TOKEN npx --yes renovate --platform=local; \
	else \
		echo "Warning: GH_ACCESS_TOKEN not set, some dependency lookups may fail"; \
		npx --yes renovate --platform=local; \
	fi

#deps-prune: @ Remove unused Go dependencies (writes go.mod/go.sum)
deps-prune: deps
	@go mod tidy

#deps-prune-check: @ Verify no prunable dependencies (CI gate)
deps-prune-check: deps
	@go mod tidy
	@if ! git diff --exit-code go.mod go.sum >/dev/null 2>&1; then \
		echo "ERROR: go.mod/go.sum not tidy. Run 'make deps-prune'."; \
		git checkout go.mod go.sum; \
		exit 1; \
	fi
	@echo "No prunable dependencies found."

#cleanup-runs: @ Prune old GitHub Actions runs (RETAIN_DAYS, KEEP_MINIMUM)
cleanup-runs:
	@if [ -z "$(GH_REPO)" ]; then echo "Error: GH_REPO is empty (gh CLI not authenticated)"; exit 1; fi
	@CUTOFF=$$(date -u -d "$(RETAIN_DAYS) days ago" +%Y-%m-%dT%H:%M:%SZ); \
	gh run list --repo "$(GH_REPO)" --json databaseId,createdAt --limit 200 \
	  --jq "sort_by(.createdAt) | reverse | .[$(KEEP_MINIMUM):] | .[] | select(.createdAt < \"$$CUTOFF\") | .databaseId" \
	| xargs -r -I{} gh run delete {} --repo "$(GH_REPO)"

#kind-up: @ Create KinD cluster (idempotent)
kind-up: deps
	@if kind get clusters | grep -qx "$(KIND_CLUSTER_NAME)"; then \
		echo "KinD cluster '$(KIND_CLUSTER_NAME)' already exists"; \
	else \
		kind create cluster --name $(KIND_CLUSTER_NAME) --wait 90s; \
	fi

#kind-down: @ Delete KinD cluster (idempotent)
kind-down:
	@kind delete cluster --name $(KIND_CLUSTER_NAME) || true

#dapr-install: @ Install Dapr control plane via Helm
dapr-install:
	@helm repo add dapr https://dapr.github.io/helm-charts/ --force-update >/dev/null
	@helm repo update >/dev/null
	@$(HELM_E2E) upgrade --install dapr dapr/dapr \
		--namespace dapr-system --create-namespace \
		--version $(DAPR_HELM_VERSION) \
		--set global.mtls.enabled=false \
		--wait

#dapr-uninstall: @ Uninstall Dapr control plane
dapr-uninstall:
	@$(HELM_E2E) uninstall dapr --namespace dapr-system || true

#e2e-redis-deploy: @ Deploy upstream redis (standalone) into the kind cluster
e2e-redis-deploy:
	@$(KUBECTL_E2E) create namespace $(APP_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL_E2E) apply -f -
	@if ! $(KUBECTL_E2E) get secret redis -n $(APP_NAMESPACE) >/dev/null 2>&1; then \
		printf '%s' "$$(openssl rand -base64 16)" | \
			$(KUBECTL_E2E) create secret generic redis -n $(APP_NAMESPACE) \
				--from-file=redis-password=/dev/stdin \
				--dry-run=client -o yaml | \
			$(KUBECTL_E2E) apply -f -; \
	fi
	@$(KUBECTL_E2E) apply -f deploy/redis.yaml -n $(APP_NAMESPACE)
	@$(KUBECTL_E2E) rollout status deployment/redis -n $(APP_NAMESPACE) --timeout=120s

#e2e-apply: @ Apply Dapr config and deployments into the kind cluster (patches imagePullPolicy for ko-loaded images)
e2e-apply:
	@$(KUBECTL_E2E) create namespace $(APP_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL_E2E) apply -f -
	@$(KUBECTL_E2E) apply -f .dapr/configuration.yaml -n $(APP_NAMESPACE)
	@# Apply pubsub/state/resiliency components but skip the chaos-demo
	@# `dummy-sub.yaml` + `dummy-resiliency.yaml` — `dummy-sub` subscribes
	@# timeline-app to the same topic (`todos`) as `timeline-sub` but routes
	@# to a non-existent path, which conflicts with the legitimate subscription
	@# in Dapr 1.17 and silently drops the real events. Project keeps the
	@# files for manual chaos-demo runs; e2e uses the legitimate path only.
	@for f in .dapr/components/*.yaml; do \
		case "$$(basename "$$f")" in dummy-*.yaml) continue;; esac; \
		$(KUBECTL_E2E) apply -f "$$f" -n $(APP_NAMESPACE) || exit 1; \
	done
	@# deploy/*.yaml ship with `imagePullPolicy: Always` for prod (force-refresh
	@# of :latest from GHCR). For e2e the images are kind-loaded locally — Always
	@# triggers an unwanted GHCR pull (and 403 if the package doesn't exist yet).
	@# Patch to `IfNotPresent` at apply-time so KinD uses the loaded copies.
	@# Skip deploy/redis.yaml — already applied by `e2e-redis-deploy`; re-applying
	@# a patched copy would trigger a Recreate-strategy restart of Redis mid-flow.
	@tmpdir=$$(mktemp -d) && trap 'rm -rf "$$tmpdir"' EXIT && \
		for f in deploy/*.yaml; do \
			case "$$(basename "$$f")" in redis.yaml) continue;; esac; \
			sed 's/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/g' "$$f" > "$$tmpdir/$$(basename "$$f")"; \
		done && \
		$(KUBECTL_E2E) apply -f "$$tmpdir" -n $(APP_NAMESPACE)

#e2e-load-images: @ ko-build images locally and `kind load` each into the cluster
e2e-load-images:
	@# Build with the SAME repo/tag the deploy YAMLs reference so locally-loaded
	@# images satisfy the spec without a YAML rewrite. `ko build --local`
	@# tags the local Docker daemon; `kind load docker-image` then ships
	@# each ref into every node of the named cluster.
	@# (Note: `ko publish --push=false` does NOT load anywhere — it builds
	@# layers in cache but never tags the daemon. ko's `kind.local`
	@# auto-loading only fires when KO_DOCKER_REPO starts with `kind.local`.
	@# Neither is what we want — see commit 9f6a928 for the failure mode.)
	@for d in $(BINARY_DIRS); do \
		ref="$(KO_DOCKER_REPO)/$$d:latest"; \
		echo "==> ko build --local $$ref"; \
		ko build --local --base-import-paths --tags=latest ./cmd/$$d >/dev/null || exit 1; \
		echo "==> kind load docker-image $$ref"; \
		kind load docker-image --name $(KIND_CLUSTER_NAME) "$$ref" || exit 1; \
	done

#e2e: @ Bring up the full stack (KinD + Dapr + Redis + ko-loaded apps) and run the smoke test
e2e: kind-up dapr-install e2e-redis-deploy e2e-load-images e2e-apply e2e-smoke

#e2e-smoke: @ Run the smoke assertions against an already-deployed cluster
e2e-smoke: deps
	@KIND_CLUSTER_NAME=$(KIND_CLUSTER_NAME) APP_NAMESPACE=$(APP_NAMESPACE) bash scripts/e2e-smoke.sh

#ci: @ Run full CI pipeline locally (deps + static-check + test + integration-test + build)
ci: deps static-check test integration-test build
	@echo "CI passed."

#ci-run: @ Run GitHub Actions workflow locally via act
ci-run: deps
	@act push --container-architecture linux/amd64 \
		-P ubuntu-24.04=catthehacker/ubuntu:$(ACT_UBUNTU_VERSION) \
		--artifact-server-path /tmp/act-artifacts \
		--bind

.PHONY: help deps test integration-test build clean lint format format-check sec vulncheck secrets \
	trivy-fs trivy-config lint-ci shellcheck mermaid-lint static-check run update update-minor \
	image-build image-scan image-smoke-test image-sign push rollout app-logs mongo-run dapr-run zipkin-deploy \
	redis-deploy apply deploy deploy-full release \
	renovate-validate deps-prune deps-prune-check cleanup-runs ci ci-run \
	kind-up kind-down dapr-install dapr-uninstall \
	e2e-redis-deploy e2e-apply e2e-load-images e2e e2e-smoke
