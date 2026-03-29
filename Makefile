.DEFAULT_GOAL := help

APP_NAME       := dapr-go-crud-app
CURRENTTAG     := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")

# === Tool Versions (pinned) ===
GOLANGCI_VERSION  := 2.1.6
KO_VERSION        := 0.18.0
ACT_VERSION       := 0.2.86
NVM_VERSION       := 0.40.4

export KO_DOCKER_REPO := docker.io/andriykalashnykov

APP_NAMESPACE ?= crud-app

#help: @ List available tasks
help:
	@echo "Usage: make COMMAND"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-30s\033[0m - %s\n", $$1, $$2}'

#deps: @ Check and install required dependencies
deps:
	@command -v go >/dev/null 2>&1 || { echo "Error: Go is required. Install from https://go.dev/dl/"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "Error: Docker is required. Install from https://www.docker.com/"; exit 1; }
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint v$(GOLANGCI_VERSION)..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@v$(GOLANGCI_VERSION); }
	@command -v ko >/dev/null 2>&1 || { echo "Installing ko v$(KO_VERSION)..."; \
		go install github.com/google/ko@v$(KO_VERSION); }

#deps-act: @ Install act for local CI
deps-act: deps
	@command -v act >/dev/null 2>&1 || { echo "Installing act $(ACT_VERSION)..."; \
		curl -sSfL https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash -s -- -b /usr/local/bin v$(ACT_VERSION); \
	}

#test: @ Run tests
test: deps
	@go test ./...

#build: @ Build all binaries
build: deps
	@go build -o ./.bin/app ./cmd/app.go
	@go build -o ./.bin/consumer ./cmd/consumer
	@go build -o ./.bin/datagen ./cmd/datagen
	@go build -o ./.bin/dummy ./cmd/dummy
	@go build -o ./.bin/errorgen ./cmd/errorgen
	@go build -o ./.bin/publisher ./cmd/publisher
	@go build -o ./.bin/service-a ./cmd/service-a
	@go build -o ./.bin/service-b ./cmd/service-b
	@go build -o ./.bin/service-c ./cmd/service-c
	@go build -o ./.bin/timeline ./cmd/timeline

#clean: @ Remove build artifacts
clean:
	@rm -rf ./.bin

#lint: @ Run linters
lint: deps
	@golangci-lint run ./...

#run: @ Run the main app locally
run: deps
	@go run ./cmd/app.go serve -connStr dapr

#update: @ Update Go dependencies
update:
	@go get -u ./...
	@go mod tidy

#push: @ Publish all images with ko
push: deps
	@ko publish ./cmd
	@ko publish ./cmd/consumer
	@ko publish ./cmd/datagen
	@ko publish ./cmd/dummy
	@ko publish ./cmd/errorgen
	@ko publish ./cmd/publisher
	@ko publish ./cmd/service-a
	@ko publish ./cmd/service-b
	@ko publish ./cmd/service-c
	@ko publish ./cmd/timeline

#rollout: @ Restart app pods
rollout:
	@kubectl delete pod -l app=crud-app
	@kubectl delete pod -l app=timeline-app

#app-logs: @ Show crud-app container logs
app-logs:
	@kubectl logs -l app=crud-app -c crud-app

#mongo-run: @ Run MongoDB in Docker
mongo-run:
	@docker run -it -p 27017:27017 mongo:4.0-xenial

#dapr-run: @ Run app with Dapr sidecar
dapr-run:
	@dapr run --app-id crud-app --app-port 8080 --dapr-http-port 3500 ./bin/app serve -connStr dapr

#zipkin-setup: @ Deploy Zipkin and expose via skind
zipkin-setup: zipkin-deploy
	@skind expose zipkin

#zipkin-deploy: @ Deploy Zipkin to Kubernetes
zipkin-deploy:
	@kubectl create deployment zipkin --image openzipkin/zipkin
	@kubectl expose deployment zipkin --type ClusterIP --port 9411

#redis-deploy: @ Deploy Redis with Helm (standalone)
redis-deploy:
	@helm upgrade --install redis bitnami/redis -n ${APP_NAMESPACE} --set architecture=standalone

#redis-deploy-replicated: @ Deploy Redis with Helm (replication)
redis-deploy-replicated:
	@helm upgrade --install redis bitnami/redis -n ${APP_NAMESPACE} --set replica.replicaCount=1

#deploy: @ Deploy full stack to Kubernetes
deploy:
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
		git add -A && \
		git commit -a -s -m "Cut $$newtag release" && \
		git tag $$newtag && \
		git push origin $$newtag && \
		git push && \
		echo "Done."'

#renovate-bootstrap: @ Install nvm and npm for Renovate
renovate-bootstrap:
	@command -v node >/dev/null 2>&1 || { \
		echo "Installing nvm $(NVM_VERSION)..."; \
		curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v$(NVM_VERSION)/install.sh | bash; \
		export NVM_DIR="$$HOME/.nvm"; \
		[ -s "$$NVM_DIR/nvm.sh" ] && . "$$NVM_DIR/nvm.sh"; \
		nvm install --lts; \
	}

#renovate-validate: @ Validate Renovate configuration
renovate-validate: renovate-bootstrap
	@npx --yes renovate --platform=local

#ci: @ Run full CI pipeline locally
ci: deps lint test build
	@echo "CI passed."

#ci-run: @ Run GitHub Actions workflow locally using act
ci-run: deps-act
	@act push --container-architecture linux/amd64 \
		--artifact-server-path /tmp/act-artifacts

.PHONY: help deps deps-act test build clean lint run update push rollout app-logs \
	mongo-run dapr-run zipkin-setup zipkin-deploy redis-deploy redis-deploy-replicated \
	deploy apply release renovate-bootstrap renovate-validate ci ci-run
