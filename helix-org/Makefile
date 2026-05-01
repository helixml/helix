# Helix Org Makefile — always drive builds/tests/lint through make.

BINARY      := helix-org
MODULE      := github.com/helixml/helix-org
CMD_PKG     := ./cmd/$(BINARY)
BIN_DIR     := bin
PKG         ?= ./...
ARGS        ?=

GOLANGCI_LINT_VERSION := v2.11.4

GO          := go
GOFLAGS     :=
CGO_ENABLED ?= 0

export CGO_ENABLED

.PHONY: help tools build run test test-cover test-e2e-helix fmt fmt-check vet lint check ci clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Install required development tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	$(GO) install go.uber.org/mock/mockgen@latest

build: ## Build the binary into ./bin
	mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -o $(BIN_DIR)/$(BINARY) $(CMD_PKG)

run: ## Run `helix-org serve` with opinionated defaults; pass extra flags via ARGS="..."
	$(GO) run $(CMD_PKG) serve --envs-dir ./envs --db ./helix-org.db --addr :8080 $(ARGS)

test: ## Run tests (use PKG=./path/... to scope)
	CGO_ENABLED=1 $(GO) test $(GOFLAGS) -race -count=1 $(PKG)

test-e2e-helix: ## Run the //go:build integration helix-end-to-end smoke (requires HELIX_URL+HELIX_API_KEY)
	CGO_ENABLED=1 $(GO) test -tags=integration -count=1 ./tools/helixclient/... ./tools/...

test-cover: ## Run tests and produce coverage.out + coverage.html
	CGO_ENABLED=1 $(GO) test $(GOFLAGS) -race -count=1 -coverprofile=coverage.out -covermode=atomic $(PKG)
	$(GO) tool cover -func=coverage.out | tail -n 1
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Wrote coverage.html"

fmt: ## Format and organise imports
	goimports -w -local $(MODULE) .

fmt-check: ## Fail if files are not formatted (does not modify)
	@unformatted=$$(goimports -l -local $(MODULE) . | grep -v '^vendor/' || true); \
	if [ -n "$$unformatted" ]; then \
	  echo "Unformatted files (run 'make fmt'):"; echo "$$unformatted"; exit 1; \
	fi

vet: ## Run go vet
	$(GO) vet $(PKG)

lint: ## Run golangci-lint
	golangci-lint run $(PKG)

check: fmt vet lint test ## Format, vet, lint, and test (modifies files)

ci: fmt-check vet lint test ## CI-safe: checks formatting without modifying

clean: ## Remove build artefacts, kill running servers, wipe local DBs and envs
	@pkill -f '[h]elix-org serve' 2>/dev/null || true
	rm -rf $(BIN_DIR) coverage.out coverage.html envs
	rm -f *.db
