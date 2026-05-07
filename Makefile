.DEFAULT_GOAL := help

GO          ?= go
GOLANGCI    ?= golangci-lint
PKG         := github.com/RVRTelecomunicaciones/sophia-cli
BIN_DIR     := bin
BIN         := $(BIN_DIR)/sophia
BIN_COV     := $(BIN_DIR)/sophia.cov
COVDIR      := coverage
VERSION     ?= 0.1.0-dev
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE        := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -X $(PKG)/internal/bootstrap.Version=$(VERSION) \
               -X $(PKG)/internal/bootstrap.Commit=$(COMMIT) \
               -X $(PKG)/internal/bootstrap.BuildDate=$(DATE)

.PHONY: help build build-cover test vet lint coverage coverage-full test-integration clean run-doctor run-version release-check release-snapshot

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
	rm -rf $(BIN_DIR) cover.out cover.unit.out cover.bin.out cover.merged.out coverage.html $(COVDIR)

run-version: build ## Build and run sophia version
	$(BIN) version

run-doctor: build ## Build and run sophia doctor
	$(BIN) doctor

build-cover: ## Build sophia with -cover instrumentation into ./bin/sophia.cov
	@mkdir -p $(BIN_DIR)
	$(GO) build -cover -coverpkg=./... -ldflags '$(LDFLAGS)' -o $(BIN_COV) ./cmd/sophia

release-check: ## Validate .goreleaser.yaml without building
	goreleaser check

release-snapshot: ## Build a local snapshot release (no publish)
	goreleaser release --snapshot --clean --skip=publish,sign

coverage-full: ## Full project coverage (unit + binary via e2e)
	@rm -rf $(COVDIR) cover.unit.out cover.bin.out cover.merged.out
	@mkdir -p $(COVDIR)/bin-raw
	# Unit tests: standard text-format profile over all non-root packages
	$(GO) test -cover -coverpkg=./... -coverprofile=$(PWD)/cover.unit.out \
	    ./cmd/... ./internal/... || true
	# Build instrumented binary and run e2e to collect binary coverage
	$(GO) build -cover -coverpkg=./... -ldflags '$(LDFLAGS)' -o $(BIN_COV) ./cmd/sophia
	GOCOVERDIR=$(PWD)/$(COVDIR)/bin-raw \
	SOPHIA_TEST_BINARY=$(PWD)/$(BIN_COV) \
	$(GO) test -tags=e2e_smoke ./test/e2e/...
	# Convert binary covdata → text format
	$(GO) tool covdata textfmt -i=$(PWD)/$(COVDIR)/bin-raw -o=$(PWD)/cover.bin.out
	# Merge: unit text profile + binary text profile (strip duplicate mode header)
	@head -1 $(PWD)/cover.unit.out > $(PWD)/cover.merged.out
	@tail -n +2 $(PWD)/cover.unit.out >> $(PWD)/cover.merged.out
	@tail -n +2 $(PWD)/cover.bin.out >> $(PWD)/cover.merged.out
	@echo ""
	@echo "=== Binary coverage (cmd/sophia) ==="
	@$(GO) tool cover -func=$(PWD)/cover.bin.out | grep 'cmd/sophia'
	@echo ""
	@echo "=== Merged coverage total ==="
	@$(GO) tool cover -func=$(PWD)/cover.merged.out | tail -1
