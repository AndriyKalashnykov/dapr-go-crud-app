.DEFAULT_GOAL := help

APP_NAME       := dapr-go-crud-app
CURRENTTAG     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")

# === Tool Versions (pinned) ===
GOLANGCI_VERSION  := 2.1.6
KO_VERSION        := 0.18.0
ACT_VERSION       := 0.2.87
NVM_VERSION       := 0.40.4
NODE_VERSION      := 22
GVM_SHA           := dd652539fa4b771840846f8319fad303c7d0a8d2 # v1.0.22

# === Go Version Management (via gvm) ===
# Parse all unique Go versions from every go.mod in the project
GO_VERSIONS := $(shell find . -name 'go.mod' -exec grep -oP '^go \K[0-9.]+' {} \; | sort -uV)
# Primary Go version from root go.mod
GO_VERSION  := $(shell grep -oP '^go \K[0-9.]+' go.mod)

# Helper: run a command under the correct Go version
# In CI, actions/setup-go provides Go directly — gvm is not needed.
# Locally, gvm sets GOROOT/GOPATH/PATH in a subshell (does not persist across recipe lines).
# go-exec detects which environment we're in and wraps commands accordingly.
HAS_GVM := $(shell [ -s "$$HOME/.gvm/scripts/gvm" ] && echo true || echo false)
define go-exec
$(if $(filter true,$(HAS_GVM)),bash -c '. $$GVM_ROOT/scripts/gvm && gvm use go$(GO_VERSION) >/dev/null && $(1)',bash -c '$(1)')
endef

export KO_DOCKER_REPO := docker.io/andriykalashnykov

APP_NAMESPACE ?= crud-app

#help: @ List available tasks
help:
	@echo "Usage: make COMMAND"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-30s\033[0m - %s\n", $$1, $$2}'

#deps: @ Check and install required dependencies
deps:
	@if [ -z "$$CI" ] && [ ! -s "$$HOME/.gvm/scripts/gvm" ]; then \
		echo "Installing gvm (Go Version Manager)..."; \
		curl -s -S -L https://raw.githubusercontent.com/moovweb/gvm/$(GVM_SHA)/binscripts/gvm-installer | bash -s $(GVM_SHA); \
		echo ""; \
		echo "gvm installed. Please restart your shell or run:"; \
		echo "  source $$HOME/.gvm/scripts/gvm"; \
		echo "Then re-run 'make deps' to install Go $(GO_VERSION) via gvm."; \
		exit 0; \
	fi
	@if [ "$(HAS_GVM)" = "true" ]; then \
		for v in $(GO_VERSIONS); do \
			bash -c '. $$GVM_ROOT/scripts/gvm && gvm list' 2>/dev/null | grep -q "go$$v" || { \
				echo "Installing Go $$v via gvm..."; \
				bash -c '. $$GVM_ROOT/scripts/gvm && gvm install go'"$$v"' -B'; \
			}; \
		done; \
	else \
		command -v go >/dev/null 2>&1 || { echo "Error: Go required. Install gvm from https://github.com/moovweb/gvm or Go from https://go.dev/dl/"; exit 1; }; \
	fi
	@command -v docker >/dev/null 2>&1 || { echo "Error: Docker is required. Install from https://www.docker.com/"; exit 1; }
	@$(call go-exec,command -v golangci-lint) >/dev/null 2>&1 || { echo "Installing golangci-lint v$(GOLANGCI_VERSION)..."; \
		$(call go-exec,go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v$(GOLANGCI_VERSION)); }
	@$(call go-exec,command -v ko) >/dev/null 2>&1 || { echo "Installing ko v$(KO_VERSION)..."; \
		$(call go-exec,go install github.com/google/ko@v$(KO_VERSION)); }
	@command -v node >/dev/null 2>&1 || { \
		echo "Installing nvm $(NVM_VERSION) + Node $(NODE_VERSION)..."; \
		curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v$(NVM_VERSION)/install.sh | bash; \
		export NVM_DIR="$$HOME/.nvm"; \
		[ -s "$$NVM_DIR/nvm.sh" ] && . "$$NVM_DIR/nvm.sh"; \
		nvm install $(NODE_VERSION); \
	}

#deps-check: @ Show required Go versions and gvm status
deps-check:
	@echo "Go versions required: $(GO_VERSIONS)"
	@echo "Primary Go version:   $(GO_VERSION)"
	@command -v gvm >/dev/null 2>&1 && { \
		bash -c '. $$GVM_ROOT/scripts/gvm && gvm list'; \
	} || echo "gvm not installed — install from https://github.com/moovweb/gvm"

#deps-act: @ Install act for local CI
deps-act: deps
	@command -v act >/dev/null 2>&1 || { echo "Installing act $(ACT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash -s -- -b /usr/local/bin v$(ACT_VERSION); \
	}

#test: @ Run tests with race detection
test: deps
	@$(call go-exec,go test -race ./...)

#build: @ Build all binaries
build: deps
	@$(call go-exec,go build -o ./.bin/app ./cmd/app.go)
	@$(call go-exec,go build -o ./.bin/consumer ./cmd/consumer)
	@$(call go-exec,go build -o ./.bin/datagen ./cmd/datagen)
	@$(call go-exec,go build -o ./.bin/dummy ./cmd/dummy)
	@$(call go-exec,go build -o ./.bin/errorgen ./cmd/errorgen)
	@$(call go-exec,go build -o ./.bin/publisher ./cmd/publisher)
	@$(call go-exec,go build -o ./.bin/service-a ./cmd/service-a)
	@$(call go-exec,go build -o ./.bin/service-b ./cmd/service-b)
	@$(call go-exec,go build -o ./.bin/service-c ./cmd/service-c)
	@$(call go-exec,go build -o ./.bin/timeline ./cmd/timeline)

#clean: @ Remove build artifacts
clean:
	@rm -rf ./.bin

#lint: @ Run golangci-lint (includes gocritic via .golangci.yml)
lint: deps
	@$(call go-exec,golangci-lint run ./...)

#run: @ Run the main app locally
run: deps
	@$(call go-exec,go run ./cmd/app.go serve -connStr dapr)

#format: @ Format Go source files
format: deps
	@$(call go-exec,gofmt -w .)

#update: @ Update Go dependencies
update: deps
	@$(call go-exec,go get -u ./...)
	@$(call go-exec,go mod tidy)

#push: @ Publish all images with ko
push: deps
	@$(call go-exec,ko publish ./cmd)
	@$(call go-exec,ko publish ./cmd/consumer)
	@$(call go-exec,ko publish ./cmd/datagen)
	@$(call go-exec,ko publish ./cmd/dummy)
	@$(call go-exec,ko publish ./cmd/errorgen)
	@$(call go-exec,ko publish ./cmd/publisher)
	@$(call go-exec,ko publish ./cmd/service-a)
	@$(call go-exec,ko publish ./cmd/service-b)
	@$(call go-exec,ko publish ./cmd/service-c)
	@$(call go-exec,ko publish ./cmd/timeline)

#rollout: @ Restart app pods
rollout:
	@kubectl delete pod -l app=crud-app
	@kubectl delete pod -l app=timeline-app

#app-logs: @ Show crud-app container logs
app-logs:
	@kubectl logs -l app=crud-app -c crud-app

#mongo-run: @ Run MongoDB in Docker
mongo-run:
	@docker run -it -p 27017:27017 mongo:8.0

#dapr-run: @ Run app with Dapr sidecar
dapr-run:
	@dapr run --app-id crud-app --app-port 8080 --dapr-http-port 3500 ./bin/app serve -connStr dapr

#zipkin-setup: @ Deploy Zipkin and expose via skind
zipkin-setup: zipkin-deploy
	@skind expose zipkin

#zipkin-deploy: @ Deploy Zipkin to Kubernetes
zipkin-deploy:
	@kubectl create deployment zipkin --image openzipkin/zipkin:3.5
	@kubectl expose deployment zipkin --type ClusterIP --port 9411

#redis-deploy: @ Deploy Redis with Helm (standalone)
redis-deploy:
	@helm upgrade --install redis bitnami/redis -n ${APP_NAMESPACE} --set architecture=standalone

#redis-deploy-replicated: @ Deploy Redis with Helm (replication)
redis-deploy-replicated:
	@helm upgrade --install redis bitnami/redis -n ${APP_NAMESPACE} --set replica.replicaCount=1

#deploy: @ Deploy full stack to Kubernetes
deploy: push
	@kubectl create namespace ${APP_NAMESPACE} || true
	@$(MAKE) redis-deploy
	@kubectl apply -f .dapr/configuration.yaml -n ${APP_NAMESPACE}
	@kubectl apply -f .dapr/components -n ${APP_NAMESPACE}
	@kubectl apply -f deploy -n ${APP_NAMESPACE}

#apply: @ Apply Dapr config and deployments
apply:
	@kubectl apply -f .dapr/configuration.yaml -n ${APP_NAMESPACE}
	@kubectl apply -f .dapr/components -n ${APP_NAMESPACE}
	@kubectl apply -f deploy -n ${APP_NAMESPACE}

#release: @ Create and push a new tag
release:
	@bash -c 'read -p "New tag (current: $(CURRENTTAG)): " newtag && \
		echo "$$newtag" | grep -qE "^v[0-9]+\.[0-9]+\.[0-9]+$$" || { echo "Error: Tag must match vN.N.N"; exit 1; } && \
		echo -n "Create and push $$newtag? [y/N] " && read ans && [ "$${ans:-N}" = y ] && \
		echo $$newtag > ./version.txt && \
		git add version.txt && \
		git commit -a -s -m "Cut $$newtag release" && \
		git tag $$newtag && \
		git push origin $$newtag && \
		git push && \
		echo "Done."'

#renovate-validate: @ Validate Renovate configuration
renovate-validate: deps
	@npx --yes renovate --platform=local

#deps-prune: @ Remove unused Go dependencies
deps-prune: deps
	@echo "=== Dependency Pruning ==="
	@echo "--- Go: running go mod tidy ---"
	@$(call go-exec,go mod tidy)
	@echo "=== Pruning complete ==="

#deps-prune-check: @ Verify no prunable dependencies (CI gate)
deps-prune-check: deps
	@$(call go-exec,go mod tidy)
	@if ! git diff --exit-code go.mod go.sum >/dev/null 2>&1; then \
		echo "ERROR: go.mod/go.sum not tidy. Run 'make deps-prune'."; \
		git checkout go.mod go.sum; \
		exit 1; \
	fi
	@echo "No prunable dependencies found."

#ci: @ Run full CI pipeline locally
ci: deps format lint test build
	@echo "CI passed."

#ci-run: @ Run GitHub Actions workflow locally using act
ci-run: deps-act
	@act push --container-architecture linux/amd64 \
		--artifact-server-path /tmp/act-artifacts

.PHONY: help deps deps-check deps-act test build clean lint format run update push rollout app-logs \
	mongo-run dapr-run zipkin-setup zipkin-deploy redis-deploy redis-deploy-replicated \
	deploy apply release renovate-validate deps-prune deps-prune-check ci ci-run
