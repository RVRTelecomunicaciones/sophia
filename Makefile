.DEFAULT_GOAL := help

GO          ?= go
GOLANGCI    ?= golangci-lint
PKG         := github.com/RVRTelecomunicaciones/sophia-cli
BIN_DIR     := bin
BIN         := $(BIN_DIR)/sophia
VERSION     ?= 0.1.0-dev
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -X $(PKG)/internal/bootstrap.Version=$(VERSION) \
               -X $(PKG)/internal/bootstrap.Commit=$(COMMIT) \
               -X $(PKG)/internal/bootstrap.BuildDate=$(DATE)

.PHONY: help build test vet lint coverage test-integration clean run-doctor run-version

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS=":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the sophia binary into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/sophia

test: ## Run all tests
	$(GO) test ./...

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint
	$(GOLANGCI) run

coverage: ## Compute coverage for domain + application
	$(GO) test -coverprofile=cover.out ./internal/domain/... ./internal/application/...
	$(GO) tool cover -func=cover.out | tail -n 1

test-integration: ## Run opt-in integration tests against a real Docker daemon
	$(GO) test -tags=integration ./test/integration/...

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) cover.out coverage.html

run-version: build ## Build and run sophia version
	$(BIN) version

run-doctor: build ## Build and run sophia doctor
	$(BIN) doctor
