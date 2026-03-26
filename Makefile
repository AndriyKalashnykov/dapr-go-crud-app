.DEFAULT_GOAL := help

# Tool versions
KO_VERSION        := 0.17.1
GOLANGCI_VERSION  := 1.64.8

export KO_DOCKER_REPO := docker.io/andriykalashnykov

APP_NAMESPACE ?= crud-app

#help: @ List available tasks
help:
	@echo "Usage: make COMMAND"
	@echo "Commands :"
	@grep -E '[a-zA-Z\.\-]+:.*?@ .*$$' $(MAKEFILE_LIST)| tr -d '#' | awk 'BEGIN {FS = ":.*?@ "}; {printf "\033[32m%-20s\033[0m - %s\n", $$1, $$2}'

#deps: @ Check required dependencies
deps:
	@command -v go >/dev/null 2>&1 || { echo "go is not installed"; exit 1; }
	@command -v docker >/dev/null 2>&1 || { echo "docker is not installed"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "kubectl is not installed"; exit 1; }
	@command -v ko >/dev/null 2>&1 || { echo "ko is not installed"; exit 1; }
	@echo "All dependencies are installed"

#test: @ Run tests
test:
	@go test ./...

#build: @ Build all binaries
build:
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
lint:
	@golangci-lint run ./...

#run: @ Run the main app locally
run:
	@go run ./cmd/app.go serve -connStr dapr

#update: @ Update Go dependencies
update:
	@go get -u ./...
	@go mod tidy

#push: @ Publish all images with ko
push:
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

#release: @ Create a release tag (usage: make release VERSION=v1.2.3)
release:
	@if [ -z "$(VERSION)" ]; then echo "VERSION is required (e.g. make release VERSION=v1.2.3)"; exit 1; fi
	@echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "VERSION must match semver pattern vX.Y.Z"; exit 1; }
	@git tag -a $(VERSION) -m "Release $(VERSION)"
	@echo "Tagged $(VERSION). Run 'git push origin $(VERSION)' to publish."

#ci: @ Run full CI pipeline locally (lint, test, build)
ci: lint test build

.PHONY: help deps test build clean lint run update push rollout app-logs mongo-run dapr-run zipkin-setup zipkin-deploy redis-deploy redis-deploy-replicated deploy apply release ci
