GOCACHE ?= $(CURDIR)/.cache/go-build
GOLANGCI_LINT_CACHE ?= $(CURDIR)/.cache/golangci-lint

.PHONY: test lint-go test-go test-web build-web build-agents migrate

test: lint-go test-go test-web

lint-go:
	GOCACHE=$(GOCACHE) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE) golangci-lint run

test-go:
	GOCACHE=$(GOCACHE) go test ./...

test-web:
	npm --workspace apps/web test

build-web:
	npm --workspace apps/web run build

build-agents:
	go build -o node-agent ./cmd/node-agent

migrate:
	go run ./cmd/migrate -database "$${DATABASE_URL:-./local.db}" up
