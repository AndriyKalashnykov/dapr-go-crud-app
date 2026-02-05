
export KO_DOCKER_REPO=docker.io/andriykalashnykov

test:
	@go test ./...

build:
	go build -o ./.bin/app ./cmd/app.go
	go build -o ./.bin/consumer ./cmd/consumer
	go build -o ./.bin/datagen ./cmd/datagen
	go build -o ./.bin/dummy ./cmd/dummy
	go build -o ./.bin/errorgen ./cmd/errorgen
	go build -o ./.bin/publisher ./cmd/publisher
	go build -o ./.bin/service-a ./cmd/service-a
	go build -o ./.bin/service-b ./cmd/service-b
	go build -o ./.bin/service-c ./cmd/service-c
	go build -o ./.bin/timeline ./cmd/timeline

update:
	@go get -u ./...; go mod tidy

push:
	ko publish ./cmd
	ko publish ./cmd/consumer
	ko publish ./cmd/datagen
	ko publish ./cmd/dummy
	ko publish ./cmd/errorgen
	ko publish ./cmd/publisher
	ko publish ./cmd/service-a
	ko publish ./cmd/service-b
	ko publish ./cmd/service-c
	ko publish ./cmd/timeline

rollout:
	kubectl delete pod -l app=crud-app
	kubectl delete pod -l app=timeline-app

crud-app-logs:
	kubectl logs -l app=crud-app -c crud-app

run-mongo:
#-e MONGO_INITDB_ROOT_USERNAME=admin -e MONGO_INITDB_ROOT_PASSWORD=admin
	docker run -it -p 27017:27017 mongo:4.0-xenial

dapr-run:
	dapr run --app-id crud-app --app-port 8080 --dapr-http-port 3500 ./bin/app serve -connStr dapr

setup-zipkin: deploy-zipkin
	skind expose zipkin

deploy-zipkin:
	kubectl create deployment zipkin --image openzipkin/zipkin
	kubectl expose deployment zipkin --type ClusterIP --port 9411

APP_NAMESPACE ?= crud-app

deploy-redis:
	helm upgrade --install redis bitnami/redis -n ${APP_NAMESPACE} --set architecture=standalone

deploy-redis-with-replication:
	helm upgrade --install redis bitnami/redis -n ${APP_NAMESPACE} --set replica.replicaCount=1

.PHONY: deploy
deploy:
	kubectl create namespace ${APP_NAMESPACE} | true
	$(MAKE) deploy-redis
	kubectl apply -f .dapr/configuration.yaml -n ${APP_NAMESPACE}
	kubectl apply -f .dapr/components -n ${APP_NAMESPACE}
	kubectl apply -f deploy -n ${APP_NAMESPACE}

apply:
	kubectl apply -f .dapr/configuration.yaml -n ${APP_NAMESPACE}
	kubectl apply -f .dapr/components -n ${APP_NAMESPACE}
	kubectl apply -f deploy -n ${APP_NAMESPACE}
