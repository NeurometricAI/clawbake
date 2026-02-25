.PHONY: all build test lint generate run-server run-operator migrate docker-build k3d-import helm-install clean

# Go settings
GOBIN := $(shell go env GOPATH)/bin
GOOS := $(shell go env GOOS)
GOARCH := $(shell go env GOARCH)

# envtest settings
ENVTEST_K8S_VERSION ?= 1.35.0
KUBEBUILDER_ASSETS ?= $(shell setup-envtest use $(ENVTEST_K8S_VERSION) -p path 2>/dev/null)

# Image settings
IMG ?= clawbake
TAG ?= dev

all: generate build test

## Build
build: build-server build-operator

build-server:
	go build -o bin/server ./cmd/server

build-operator:
	go build -o bin/operator ./cmd/operator

## Run
DOTENV := $(or $(wildcard .env.local), $(wildcard .env.example))

run-server:
	@echo "Using environment from $(DOTENV)"
	set -a && [ -f "$(DOTENV)" ] && . ./$(DOTENV); go run ./cmd/server

run-operator:
	go run ./cmd/operator

## Test
test:
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./... -v -count=1

test-unit:
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./internal/... -v -count=1

test-integration:
	go test ./tests/integration/... -v -count=1 -tags=integration

test-e2e:
	go test ./tests/e2e/... -v -count=1 -tags=e2e

## Code generation
generate: generate-sqlc generate-crd generate-templ

generate-sqlc:
	sqlc generate -f db/sqlc.yaml

generate-crd:
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./api/..."
	controller-gen crd paths="./api/..." output:crd:artifacts:config=config/crd
	controller-gen rbac:roleName=clawbake-operator paths="./internal/operator/..." output:rbac:artifacts:config=config/rbac
	cp config/crd/*.yaml charts/clawbake/crds/

generate-templ:
	templ generate ./web/templates/...

## Lint
lint:
	golangci-lint run ./...

## Database
migrate-up:
	migrate -database "$(DATABASE_URL)" -path db/migrations up

migrate-down:
	migrate -database "$(DATABASE_URL)" -path db/migrations down 1

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir db/migrations -seq $$name

migrate-drop:
	migrate -database "$(DATABASE_URL)" -path db/migrations drop

## Docker
docker-build:
	docker build -t $(IMG)-server:$(TAG) --build-arg BINARY=server .
	docker build -t $(IMG)-operator:$(TAG) --build-arg BINARY=operator .

k3d-import: docker-build
	k3d image import $(IMG)-server:$(TAG) $(IMG)-operator:$(TAG) --cluster clawbake

## Helm
helm-install:
	kubectl apply -f charts/clawbake/crds/
	helm upgrade --install clawbake charts/clawbake \
		--namespace clawbake --create-namespace

helm-install-local:
	kubectl apply -f charts/clawbake/crds/
	helm upgrade --install clawbake charts/clawbake \
		--namespace clawbake --create-namespace \
		-f charts/clawbake/values-local.yaml

helm-template:
	helm template clawbake charts/clawbake --namespace clawbake

## K3d (local dev cluster)
k3d-create:
	k3d cluster create clawbake \
		--port "8080:80@loadbalancer" \
		--port "8443:443@loadbalancer" \
		--port "5432:30432@server:0" \
		--agents 2

k3d-delete:
	k3d cluster delete clawbake

## Port forwarding (access from host via VS Code forwardPorts)
port-forward:
	kubectl port-forward svc/clawbake-server -n clawbake 8080:80

## Clean
clean:
	rm -rf bin/
