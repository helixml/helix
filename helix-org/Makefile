# Helix Org Makefile — package-level test/lint/format only.
#
# The standalone `helix-org` CLI binary was removed in H7 — production
# deployments mount helix-org from inside `helix api`
# (api/pkg/server/helix_org.go), so there is no longer a `cmd/` to
# build or run from here. What remains is library code: tests,
# linting, formatting, coverage. Build targets that produced ./bin/helix-org
# have been removed; run the helix api binary instead.

MODULE      := github.com/helixml/helix
PKG         ?= ./...

GOLANGCI_LINT_VERSION := v2.11.4

GO          := go
GOFLAGS     :=
CGO_ENABLED ?= 0

export CGO_ENABLED

.PHONY: help tools test test-cover test-e2e-helix fmt fmt-check vet lint check ci clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

tools: ## Install required development tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	$(GO) install go.uber.org/mock/mockgen@latest

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

clean: ## Remove coverage and envs artefacts (no binary to delete any more)
	rm -rf coverage.out coverage.html envs
	rm -f *.db
